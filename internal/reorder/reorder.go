package reorder

import (
	"fmt"
	"math"

	"github.com/team-d-mm/retail-agent/internal/models"
)

const safetyBuffer = 1.5

// Recommend computes a single product's reorder recommendation.
// unitsSold90 is total units sold over the last 90 days.
func Recommend(p models.Product, unitsSold90, leadTimeDays int) models.Recommendation {
	avgDaily := float64(unitsSold90) / 90.0
	rec := models.Recommendation{Product: p, AvgDailySale: avgDaily}

	if avgDaily == 0 {
		rec.Reason = fmt.Sprintf("No recent sales for %s; no order needed.", p.Name)
		return rec
	}
	if p.StockLevel >= p.ReorderPt {
		rec.Reason = fmt.Sprintf("%s stock (%d) is above reorder point (%d); no order needed.", p.Name, p.StockLevel, p.ReorderPt)
		return rec
	}

	base := avgDaily * float64(leadTimeDays) * safetyBuffer
	want := int(math.Ceil(base)) - p.StockLevel + p.ReorderPt
	if want < 0 {
		want = 0
	}
	qty := want

	if p.ShelfLifeDays > 0 {
		sellable := avgDaily * float64(p.ShelfLifeDays)
		room := int(math.Floor(sellable)) - p.StockLevel
		if room < 0 {
			room = 0
		}
		if qty > room {
			qty = room
			rec.SpoilageRisk = true
		}
		if float64(p.StockLevel) > sellable {
			rec.SpoilageRisk = true
		}
	}

	rec.ReorderQty = qty
	if rec.SpoilageRisk {
		rec.Reason = fmt.Sprintf("Sells ~%.1f/day, %d-day lead time; capped at %d so it sells within its %d-day shelf life.", avgDaily, leadTimeDays, qty, p.ShelfLifeDays)
	} else {
		rec.Reason = fmt.Sprintf("Sells ~%.1f/day, %d-day lead time; order %d to cover demand plus buffer.", avgDaily, leadTimeDays, qty)
	}
	return rec
}
