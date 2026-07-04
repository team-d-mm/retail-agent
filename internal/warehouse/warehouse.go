package warehouse

import (
	"context"
	"fmt"

	"github.com/team-d-mm/retail-agent/internal/models"
)

var errNotImplemented = fmt.Errorf("warehouse: not implemented")

type Store interface {
	GetProducts(ctx context.Context) ([]models.Product, error)
	GetSales(ctx context.Context, productID string) ([]models.Sale, error)
	GetSuppliers(ctx context.Context) ([]models.Supplier, error)
	GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error)
	GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error)
}

// Client is the legacy placeholder; the BigQuery implementation is added in Task 4.
type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) GetProducts(ctx context.Context) ([]models.Product, error) { return nil, errNotImplemented }
func (c *Client) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	return nil, errNotImplemented
}
func (c *Client) GetSuppliers(ctx context.Context) ([]models.Supplier, error) { return nil, errNotImplemented }
func (c *Client) GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error) {
	return nil, errNotImplemented
}
func (c *Client) GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error) {
	return nil, errNotImplemented
}
