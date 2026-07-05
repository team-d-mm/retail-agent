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
