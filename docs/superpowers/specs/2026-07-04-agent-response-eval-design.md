# Agent Response Evaluation — Design

**Date:** 2026-07-04
**Goal:** Evaluate the LLM agent's responses with a few focused test cases so regressions in correctness (wrong items, no tool use, missing spoilage insight) are caught automatically.
**Scope:** A small production helper (`agentrun.RunTrace`) plus a new Go eval test package. No change to the agents, tools, reorder math, or BigQuery access.

## Why this shape

ADK's Python evaluation harness (`agents-cli eval run`, `evalset.json`, built-in metrics) does not apply — this is a Go project with no `agents-cli`. But the ADK eval *methodology* transfers directly. We implement three of its dimensions as Go tests:

1. **Tool trajectory** — did the orchestrator actually delegate to the data tools (not answer from thin air / only Google Search)?
2. **Response correctness vs. ground truth** — because the reorder decision is computed deterministically by `internal/reorder`, we know the *correct* answer for a seeded catalog and can check the LLM's narrative against it (deterministic keyword/set assertions).
3. **LLM-as-judge quality** — a second Gemini call scores the narrative against a rubric (grounded in the tool data, recommends the correct items, mentions the spoilage insight, actionable), returning a 0–1 score checked against a threshold. This is the ADK `rubric_based_final_response_quality_v1` idea, robust to phrasing.

Dimensions 1–2 are cheap and near-deterministic; dimension 3 costs an extra model call and is graded to a threshold rather than exact-match.

## Ground-truth scenario

A single in-memory `warehouse.FakeStore` seeded with a known catalog, chosen so the expected outcome is unambiguous:

| Product | Perishable? | Stock vs reorder point | 90-day sales | Expected |
|---------|-------------|------------------------|--------------|----------|
| Milk (P1) | yes, `shelf_life_days=4` | below (5 < 20) | high (~10/day) | **reorder, spoilage-capped** |
| Bread (P2) | yes, `shelf_life_days=3` | below (8 < 25) | high (~9/day) | **reorder, spoilage-capped** |
| Rice (P3) | no | above (100 ≥ 20) | steady | **no order** |

Supplier `S1` with `lead_time_days=3`. The deterministic ground truth = `reorder.RecommendAll(ctx, store)`; for this seed it yields Milk and Bread (both `SpoilageRisk = true`) and excludes Rice. The eval asserts the *agent's narrative* agrees with this computed set — the LLM output is graded against code, not against a hand-written expected string.

## Components

### 1. `agentrun.RunTrace` (new, small)

Today `agentrun.Run` returns only the final narrative text, so a test cannot see which tools were called. Add a trace-returning variant and have `Run` delegate to it:

```go
type Result struct {
    Narrative string   // concatenated final-response text
    ToolCalls []string // names of function/tool calls seen across the run, in order
}

func RunTrace(ctx context.Context, apiKey string, store warehouse.Store) (Result, error)
func Run(ctx context.Context, apiKey string, store warehouse.Store) (string, error) // calls RunTrace, returns Result.Narrative
```

`RunTrace` runs the orchestrator exactly as `Run` does, but while iterating events it also collects tool-call names. In ADK a tool/function call appears as a `genai.Part` with a non-nil `FunctionCall`; the run loop records `part.FunctionCall.Name` for every event (not only final-response events). This keeps `Run`'s existing behavior byte-for-byte (it returns `Result.Narrative`).

### 2. `internal/eval/judge.go` (new) — LLM-as-judge helper

A small non-test helper so the grading call is reusable and testable:

```go
type Verdict struct {
    Score  float64 // 0.0–1.0
    Reason string
}

// Judge asks a Gemini model to score an agent narrative against a rubric,
// given the ground-truth facts it should reflect. Returns the parsed verdict.
func Judge(ctx context.Context, apiKey, narrative, rubric, groundTruth string) (Verdict, error)
```

`Judge` builds a `genai` client with the API key, sends a grading prompt that embeds the rubric, the computed ground-truth reorder facts, and the agent's narrative, and instructs the judge to reply with strict JSON `{"score": <0..1>, "reason": "..."}`. It parses that JSON (tolerating a ```json fenced block) into a `Verdict`. The judge model is `gemini-2.5-flash`. To reduce judge non-determinism the grading prompt asks for a conservative, criteria-by-criteria assessment; the threshold (below) absorbs residual variance.

### 3. `internal/eval/eval_test.go` (new)

Table-driven Go test. Each case builds the seeded `FakeStore`, calls `agentrun.RunTrace` once (shared/cached across the file so the agent is invoked a single time), and asserts. The whole file `t.Skip`s when `GOOGLE_API_KEY` is unset (consistent with `internal/agent` and `internal/agentrun` tests).

Deterministic assertions (case-insensitive substring / set membership — tolerant of phrasing):

- **`TestEval_UsesDataTools`** — `Result.ToolCalls` contains at least one of the retail data tools (`get_product_insights`, `check_stock`, `pick_supplier`). Fails if the agent never delegated to a data tool.
- **`TestEval_RecommendsCorrectItems`** — for every product in the deterministic ground-truth reorder set (Milk, Bread), the narrative contains that product name. Catches the agent dropping a needed reorder.
- **`TestEval_NoFalseOrders`** — the narrative does not instruct reordering the above-threshold staple: it must not contain "rice" in an order context. Implemented conservatively as "narrative does not contain the substring `rice`" for this seed (Rice is the only non-reorder item and shares no substring with the reorder items). Documented as seed-specific.
- **`TestEval_MentionsSpoilageInsight`** — narrative contains `spoil` or `shelf life` (the grocery-specific insight that distinguishes this agent).

LLM-as-judge assertion:

- **`TestEval_JudgeQuality`** — builds the ground-truth string from `reorder.RecommendAll` (the products, quantities, and which are spoilage-capped), calls `eval.Judge` with a rubric covering *grounded in the data / recommends exactly the correct items / mentions the spoilage cap / actionable for a shop owner*, and asserts `Verdict.Score >= 0.7`. On failure it prints the score, the judge's reason, and the narrative.

Each assertion failure prints the offending narrative + tool-call list so a human can see what the model actually said.

## Error handling

- No `GOOGLE_API_KEY` → skip (the eval needs the live model; not a failure).
- `RunTrace` returns an error → `t.Fatalf` (infrastructure failure, distinct from a content assertion failure).
- Empty narrative with no error → the content assertions fail with the captured (empty) output shown.
- `Judge` returns an error, or its reply is not parseable JSON → `t.Fatalf` in the judge case (grading infrastructure failure, distinct from a low score). A successfully parsed low score is an assertion failure, not a fatal.

## Testing / proving it works

- `go build ./... && go vet ./... && go test ./...` stays green with no key (eval skips; everything else runs).
- With `GOOGLE_API_KEY` set, `go test ./internal/eval/ -v` runs the cases against live Gemini; the run output (narrative + tool calls + judge score/reason + pass/fail per case) is the evidence. The run makes one agent invocation plus one judge call.
- Because the model is non-deterministic, the deterministic assertions are inclusion/among-set (never exact-string) and the judge case is threshold-based; this is the ADK-recommended way to keep response evals from flaking.
- The provided `GOOGLE_API_KEY` (in the git-ignored `.env`) can be used to run the eval locally: `source .env && go test ./internal/eval/ -v`.

## Out of scope

- Multi-turn conversation evals (the product is single-shot: one button, one fixed prompt).
- A standalone `agents-cli`/`evalset.json` harness (not available for Go ADK).
- Any change to agent instructions, tools, or the reorder logic. If an eval fails, the fix is in those files — but this design only adds the eval.
