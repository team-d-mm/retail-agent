# Agent Response Evaluation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add eval test cases that check the LLM agent's response for correctness — tool trajectory, response-vs-ground-truth, and an LLM-as-judge quality score.

**Architecture:** Expose the agent run's tool trajectory via `agentrun.RunTrace`, add an `eval.Judge` LLM-as-judge helper, and add `internal/eval/eval_test.go` with five cases that run the orchestrator once against a seeded `FakeStore` and assert against the deterministic `reorder.RecommendAll` ground truth. Live-model tests skip without `GOOGLE_API_KEY`.

**Tech Stack:** Go, Google ADK v1.5.0 (`runner`), `google.golang.org/genai` v1.57.0 (judge call + tool-call parts).

## Global Constraints

- Module path: `github.com/team-d-mm/retail-agent`. Go 1.26.4.
- Model id verbatim: `gemini-2.5-flash`.
- genai client (verbatim): `genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})`; `client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(prompt), nil)`; read text with `resp.Text()`.
- Tool-call name from an event part: `part.FunctionCall != nil` → `part.FunctionCall.Name`.
- Live-model tests `t.Skip` when `GOOGLE_API_KEY` is unset.
- Assertions on model text are case-insensitive inclusion / set membership, never exact-string; the judge case is threshold-based (`>= 0.7`).
- `agentrun.Run(ctx, apiKey, store) (string, error)` must keep its existing behavior (returns the narrative).
- CI must stay green: `go build ./... && go vet ./... && go test ./...`.
- The real `GOOGLE_API_KEY` is in the git-ignored `.env`; run the live eval with `source .env && go test ./internal/eval/ -v`.

---

## File Structure

- `internal/agentrun/agentrun.go` (modify) — add `Result` type + `RunTrace`; make `Run` delegate to `RunTrace`.
- `internal/agentrun/agentrun_test.go` (modify) — add a `RunTrace` smoke test (skips without key).
- `internal/eval/judge.go` (create) — `Verdict`, `Judge`, and an offline-testable `parseVerdict`.
- `internal/eval/judge_test.go` (create) — offline unit tests for `parseVerdict`.
- `internal/eval/eval_test.go` (create) — the five agent-response eval cases.

---

## Task 1: `agentrun.RunTrace` (expose tool trajectory)

**Files:**
- Modify: `internal/agentrun/agentrun.go`
- Test: `internal/agentrun/agentrun_test.go`

**Interfaces:**
- Produces: `agentrun.Result{Narrative string, ToolCalls []string}`; `agentrun.RunTrace(ctx context.Context, apiKey string, store warehouse.Store) (Result, error)`.
- Consumes: `agent.New` (existing), ADK `runner`/`session`, `genai`.

- [ ] **Step 1: Write the failing test**

Add to `internal/agentrun/agentrun_test.go`:
```go
func TestRunTraceReturnsNarrativeAndTools(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	store := &warehouse.FakeStore{
		Products:       []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers:      []models.Supplier{{ID: "S1", Name: "Acme", Reliability: 0.9, LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{"P1": {{ProductID: "P1", Quantity: 900}}},
	}
	res, err := agentrun.RunTrace(context.Background(), os.Getenv("GOOGLE_API_KEY"), store)
	if err != nil {
		t.Fatalf("RunTrace error: %v", err)
	}
	if res.Narrative == "" {
		t.Error("expected non-empty narrative")
	}
	// ToolCalls may legitimately be empty on a given run; just assert the field is usable.
	t.Logf("tool calls: %v", res.ToolCalls)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agentrun/ -run TestRunTrace -v`
Expected: FAIL — `agentrun.RunTrace` / `agentrun.Result` undefined (compile error).

- [ ] **Step 3: Write minimal implementation**

In `internal/agentrun/agentrun.go`, add the `Result` type and `RunTrace`, and rewrite `Run` to delegate. Keep the existing imports (`context`, `strings`, `uuid`, `adk`, `runner`, `session`, `genai`, `agent`, `warehouse`) and the `fixedPrompt` constant:
```go
// Result holds a run's final narrative text and the tool-call trajectory.
type Result struct {
	Narrative string
	ToolCalls []string
}

// RunTrace executes the orchestrator with the fixed prompt and returns both the
// concatenated final-response text and the names of tool calls made along the way.
func RunTrace(ctx context.Context, apiKey string, store warehouse.Store) (Result, error) {
	root, err := agent.New(ctx, apiKey, store)
	if err != nil {
		return Result{}, err
	}
	r, err := runner.New(runner.Config{
		AppName:           "retail_agent",
		Agent:             root,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return Result{}, err
	}

	msg := genai.NewContentFromText(fixedPrompt, genai.RoleUser)
	var sb strings.Builder
	var toolCalls []string
	for ev, err := range r.Run(ctx, "web-user", uuid.NewString(), msg, adk.RunConfig{}) {
		if err != nil {
			return Result{}, err
		}
		if ev.LLMResponse.Content != nil {
			for _, p := range ev.LLMResponse.Content.Parts {
				if p.FunctionCall != nil {
					toolCalls = append(toolCalls, p.FunctionCall.Name)
				}
			}
		}
		if ev.IsFinalResponse() && ev.LLMResponse.Content != nil {
			for _, p := range ev.LLMResponse.Content.Parts {
				if p.Text != "" {
					sb.WriteString(p.Text)
				}
			}
		}
	}
	return Result{Narrative: sb.String(), ToolCalls: toolCalls}, nil
}

// Run executes the orchestrator and returns the narrative text.
func Run(ctx context.Context, apiKey string, store warehouse.Store) (string, error) {
	res, err := RunTrace(ctx, apiKey, store)
	return res.Narrative, err
}
```
Delete the old body of `Run` (the runner loop now lives in `RunTrace`).

- [ ] **Step 4: Run build + tests to verify**

Run: `go build ./... && go vet ./... && go test ./internal/agentrun/ -v`
Expected: build + vet clean; both agentrun tests PASS with a key, SKIP without.

- [ ] **Step 5: Commit**

```bash
git add internal/agentrun/
git commit -m "feat(agentrun): RunTrace exposes tool-call trajectory"
```

---

## Task 2: `eval.Judge` (LLM-as-judge)

**Files:**
- Create: `internal/eval/judge.go`
- Test: `internal/eval/judge_test.go`

**Interfaces:**
- Produces: `eval.Verdict{Score float64, Reason string}`; `eval.Judge(ctx context.Context, apiKey, narrative, rubric, groundTruth string) (Verdict, error)`; internal `parseVerdict(string) (Verdict, error)`.
- Consumes: `google.golang.org/genai`.

- [ ] **Step 1: Write the failing test** (offline — tests the parser, no API)

Create `internal/eval/judge_test.go`:
```go
package eval

import "testing"

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    float64
		wantErr bool
	}{
		{"plain", `{"score": 0.9, "reason": "good"}`, 0.9, false},
		{"fenced", "```json\n{\"score\": 0.5, \"reason\": \"meh\"}\n```", 0.5, false},
		{"prose around", "Here is my verdict:\n{\"score\": 0.75, \"reason\": \"ok\"}\nThanks", 0.75, false},
		{"no json", "I cannot grade this.", 0, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseVerdict(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", v)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.Score != tt.want {
				t.Errorf("Score = %v, want %v", v.Score, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/eval/ -run TestParseVerdict -v`
Expected: FAIL — package `eval` / `parseVerdict` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/eval/judge.go`:
```go
// Package eval contains agent-response evaluation helpers and tests.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// Verdict is an LLM judge's score for an agent response.
type Verdict struct {
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// Judge asks a Gemini model to score an agent narrative against a rubric, given
// the ground-truth facts the narrative should reflect.
func Judge(ctx context.Context, apiKey, narrative, rubric, groundTruth string) (Verdict, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return Verdict{}, fmt.Errorf("judge: new client: %w", err)
	}
	prompt := fmt.Sprintf(`You are a strict evaluator of a retail assistant's answer.

GROUND TRUTH (the correct reorder facts, computed deterministically):
%s

RUBRIC (score how well the answer meets ALL of these):
%s

AGENT ANSWER:
%s

Assess each rubric point, then give an overall score from 0.0 (fails the rubric) to 1.0 (fully meets it).
Reply with STRICT JSON only, no prose, in exactly this form:
{"score": <number between 0 and 1>, "reason": "<one sentence>"}`, groundTruth, rubric, narrative)

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(prompt), nil)
	if err != nil {
		return Verdict{}, fmt.Errorf("judge: generate: %w", err)
	}
	return parseVerdict(resp.Text())
}

// parseVerdict extracts a Verdict from a model reply, tolerating ```json fences
// and surrounding prose.
func parseVerdict(s string) (Verdict, error) {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j < 0 || j < i {
		return Verdict{}, fmt.Errorf("no JSON object in judge reply: %q", s)
	}
	var v Verdict
	if err := json.Unmarshal([]byte(s[i:j+1]), &v); err != nil {
		return Verdict{}, fmt.Errorf("parse judge reply: %w", err)
	}
	return v, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/eval/ -run TestParseVerdict -v && go build ./...`
Expected: all four parser cases PASS; build clean (confirms the genai judge call compiles).

- [ ] **Step 5: Commit**

```bash
git add internal/eval/judge.go internal/eval/judge_test.go
git commit -m "feat(eval): LLM-as-judge helper with offline-tested parser"
```

---

## Task 3: Agent-response eval cases

**Files:**
- Create: `internal/eval/eval_test.go`

**Interfaces:**
- Consumes: `agentrun.RunTrace`/`Result` (Task 1), `eval.Judge`/`Verdict` (Task 2), `reorder.RecommendAll`, `warehouse.FakeStore`, `models`.

- [ ] **Step 1: Write the eval cases**

Create `internal/eval/eval_test.go`:
```go
package eval_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/agentrun"
	"github.com/team-d-mm/retail-agent/internal/eval"
	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

// seededStore: Milk & Bread must be reordered (perishable, below reorder point,
// spoilage-capped); Rice is above its reorder point, so it must NOT be ordered.
func seededStore() *warehouse.FakeStore {
	return &warehouse.FakeStore{
		Products: []models.Product{
			{ID: "P1", Name: "Milk", Category: "dairy", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4},
			{ID: "P2", Name: "Bread", Category: "bakery", SupplierID: "S1", StockLevel: 8, ReorderPt: 25, ShelfLifeDays: 3},
			{ID: "P3", Name: "Rice", Category: "staple", SupplierID: "S1", StockLevel: 100, ReorderPt: 20},
		},
		Suppliers: []models.Supplier{{ID: "S1", Name: "Acme", Reliability: 0.95, LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{
			"P1": {{ProductID: "P1", Quantity: 900}},
			"P2": {{ProductID: "P2", Quantity: 810}},
			"P3": {{ProductID: "P3", Quantity: 270}},
		},
	}
}

var (
	once   sync.Once
	shared agentrun.Result
	sharedErr error
)

// evalRun invokes the agent once for the whole package (skips without a key).
func evalRun(t *testing.T) agentrun.Result {
	t.Helper()
	key := os.Getenv("GOOGLE_API_KEY")
	if key == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	once.Do(func() {
		shared, sharedErr = agentrun.RunTrace(context.Background(), key, seededStore())
	})
	if sharedErr != nil {
		t.Fatalf("RunTrace failed: %v", sharedErr)
	}
	return shared
}

func TestEval_UsesDataTools(t *testing.T) {
	res := evalRun(t)
	dataTools := map[string]bool{"get_product_insights": true, "check_stock": true, "pick_supplier": true}
	used := false
	for _, name := range res.ToolCalls {
		if dataTools[name] {
			used = true
			break
		}
	}
	if !used {
		t.Errorf("agent never called a data tool; tool calls = %v\nnarrative:\n%s", res.ToolCalls, res.Narrative)
	}
}

func TestEval_RecommendsCorrectItems(t *testing.T) {
	res := evalRun(t)
	lower := strings.ToLower(res.Narrative)
	for _, want := range []string{"milk", "bread"} {
		if !strings.Contains(lower, want) {
			t.Errorf("narrative missing must-reorder item %q\nnarrative:\n%s", want, res.Narrative)
		}
	}
}

func TestEval_NoFalseOrders(t *testing.T) {
	res := evalRun(t)
	// Rice is above its reorder point for this seed; it must not appear as an order.
	if strings.Contains(strings.ToLower(res.Narrative), "rice") {
		t.Errorf("narrative mentions Rice, which should not be reordered\nnarrative:\n%s", res.Narrative)
	}
}

func TestEval_MentionsSpoilageInsight(t *testing.T) {
	res := evalRun(t)
	lower := strings.ToLower(res.Narrative)
	if !strings.Contains(lower, "spoil") && !strings.Contains(lower, "shelf life") {
		t.Errorf("narrative omits the spoilage/shelf-life insight\nnarrative:\n%s", res.Narrative)
	}
}

func TestEval_JudgeQuality(t *testing.T) {
	res := evalRun(t)
	key := os.Getenv("GOOGLE_API_KEY")

	recs, err := reorder.RecommendAll(context.Background(), seededStore())
	if err != nil {
		t.Fatalf("ground truth: %v", err)
	}
	var gt strings.Builder
	for _, r := range recs {
		cap := ""
		if r.SpoilageRisk {
			cap = " (spoilage-capped)"
		}
		fmt.Fprintf(&gt, "- %s: order %d units%s\n", r.Product.Name, r.ReorderQty, cap)
	}
	gt.WriteString("- Rice: no order needed (stock above reorder point)\n")

	rubric := strings.Join([]string{
		"1. Recommends reordering exactly the ground-truth items (Milk and Bread) and no others.",
		"2. Does NOT recommend reordering Rice.",
		"3. Mentions that perishable orders are capped to avoid spoilage / shelf life.",
		"4. Is clear and actionable for a small shop owner.",
	}, "\n")

	v, err := eval.Judge(context.Background(), key, res.Narrative, rubric, gt.String())
	if err != nil {
		t.Fatalf("judge failed: %v", err)
	}
	t.Logf("judge score=%.2f reason=%s", v.Score, v.Reason)
	if v.Score < 0.7 {
		t.Errorf("judge score %.2f below 0.70\nreason: %s\nnarrative:\n%s", v.Score, v.Reason, res.Narrative)
	}
}
```

- [ ] **Step 2: Verify it compiles and skips cleanly without a key**

Run: `go build ./... && go vet ./... && go test ./internal/eval/ -v`
Expected: build + vet clean; `TestParseVerdict` PASSES; the five `TestEval_*` cases SKIP (no key in this shell).

- [ ] **Step 3: Run the live eval with the provided key (evidence)**

Run: `source .env && go test ./internal/eval/ -v`
Expected: `TestParseVerdict` passes; the five `TestEval_*` cases run against live Gemini. Paste the output (tool calls, judge score/reason, pass/fail per case) into the task report as evidence.

> If a case fails, that is a real signal about the agent, not the eval. Per the eval methodology, diagnose and fix the agent (instructions/tools) — do NOT weaken the assertion or lower the judge threshold to force a pass. Report a genuine failure with its output rather than hiding it. Note: the deterministic reorder recommendations are unchanged by this task; only agent prose/tool-use behavior can move these scores.

- [ ] **Step 4: Commit**

```bash
git add internal/eval/eval_test.go
git commit -m "test(eval): agent-response eval cases (trajectory, correctness, judge)"
```

---

## Self-Review

**Spec coverage:**
- `agentrun.RunTrace` exposing tool calls → Task 1. ✅
- LLM-as-judge (`eval.Judge` + `Verdict` + parser) → Task 2. ✅
- Seeded ground-truth scenario (Milk/Bread reorder, Rice no-order) → Task 3 `seededStore`. ✅
- Five cases: UsesDataTools, RecommendsCorrectItems, NoFalseOrders, MentionsSpoilageInsight, JudgeQuality → Task 3. ✅
- Skips without `GOOGLE_API_KEY`; runs live with `.env` → Task 3 Steps 2–3. ✅
- `Run` behavior preserved → Task 1 Step 3 (delegates to RunTrace, returns Narrative). ✅

**Placeholder scan:** No TBD/TODO; every code step is complete. ✅

**Type/name consistency:** `agentrun.Result{Narrative, ToolCalls}` and `RunTrace` used identically in Tasks 1/3. `eval.Verdict{Score, Reason}`, `eval.Judge`, `parseVerdict` consistent in Tasks 2/3. Data-tool names (`get_product_insights`, `check_stock`, `pick_supplier`) match the tools built in `internal/tools`. `reorder.RecommendAll(ctx, store)` signature matches its definition. `genai` calls match the Global Constraints. ✅
