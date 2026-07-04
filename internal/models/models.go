package models

type Product struct {
	ID            string
	Name          string
	Category      string
	SupplierID    string
	UnitPrice     float64
	StockLevel    int
	ReorderPt     int
	ShelfLifeDays int
}

type Sale struct {
	ProductID string
	Quantity  int
	Total     float64
	Date      string
}

type Supplier struct {
	ID             string
	Name           string
	Reliability    float64
	LeadTimeDays   int
	AvgUnitPrice   float64
}

type ProductInsight struct {
	Product      Product
	AvgDailySale float64
	Forecast     int
	RecommendQty int
}

type Recommendation struct {
	Product      Product
	AvgDailySale float64
	ReorderQty   int
	SpoilageRisk bool
	Reason       string
}

type DailySales struct {
	Date     string
	Quantity int
}

type TopSeller struct {
	ProductID string
	Name      string
	Units     int
}
