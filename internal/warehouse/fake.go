package warehouse

import (
	"context"

	"github.com/team-d-mm/retail-agent/internal/models"
)

// FakeStore is an in-memory Store for tests.
type FakeStore struct {
	Products       []models.Product
	Suppliers      []models.Supplier
	SalesByProduct map[string][]models.Sale
	Trend          []models.DailySales
	TopSellers     []models.TopSeller
	Err            error
}

func (f *FakeStore) GetProducts(ctx context.Context) ([]models.Product, error) {
	return f.Products, f.Err
}
func (f *FakeStore) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.SalesByProduct[productID], nil
}
func (f *FakeStore) GetSuppliers(ctx context.Context) ([]models.Supplier, error) {
	return f.Suppliers, f.Err
}
func (f *FakeStore) GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error) {
	return f.Trend, f.Err
}
func (f *FakeStore) GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error) {
	return f.TopSellers, f.Err
}
