package warehouse

import (
	"context"
	"fmt"

	"github.com/dannykhant/retail-agent/internal/models"
)

type Client struct{}

func New() *Client {
	return &Client{}
}

func (c *Client) GetProducts(ctx context.Context) ([]models.Product, error) {
	return nil, fmt.Errorf("warehouse: GetProducts not implemented")
}

func (c *Client) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	return nil, fmt.Errorf("warehouse: GetSales not implemented")
}

func (c *Client) GetSuppliers(ctx context.Context) ([]models.Supplier, error) {
	return nil, fmt.Errorf("warehouse: GetSuppliers not implemented")
}
