package eval_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
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
	once      sync.Once
	shared    agentrun.Result
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
	// Rice is above its reorder point for this seed; it must not be recommended
	// for reorder. Mentioning Rice's current stock is fine — only ordering
	// language near "rice" is a failure.
	lower := strings.ToLower(res.Narrative)
	orderRice := regexp.MustCompile(`(order|reorder|restock|buy|purchase)[^.\n]*rice|rice[^.\n]*(order|reorder|restock|buy|purchase)`)
	if orderRice.MatchString(lower) {
		t.Errorf("narrative appears to recommend ordering Rice, which is above its reorder point\nnarrative:\n%s", res.Narrative)
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
		spoilageCap := ""
		if r.SpoilageRisk {
			spoilageCap = " (spoilage-capped)"
		}
		fmt.Fprintf(&gt, "- %s: order %d units%s\n", r.Product.Name, r.ReorderQty, spoilageCap)
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
