package warehouse

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

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

type DatasetRef struct {
	Project string
	Dataset string
}

// ParseDatasetURL accepts "project.dataset", "project:dataset", or "bq://project/dataset".
func ParseDatasetURL(s string) (DatasetRef, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "bq://")
	var parts []string
	switch {
	case strings.Contains(s, "/"):
		parts = strings.SplitN(s, "/", 2)
	case strings.Contains(s, ":"):
		parts = strings.SplitN(s, ":", 2)
	case strings.Contains(s, "."):
		parts = strings.SplitN(s, ".", 2)
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return DatasetRef{}, fmt.Errorf("warehouse: cannot parse dataset URL %q (want project.dataset)", s)
	}
	return DatasetRef{Project: parts[0], Dataset: parts[1]}, nil
}

type BQStore struct {
	client *bigquery.Client
	ref    DatasetRef
}

// NewBigQuery builds a BigQuery-backed Store from a service-account JSON and dataset URL.
func NewBigQuery(ctx context.Context, credsJSON []byte, datasetURL string) (*BQStore, error) {
	ref, err := ParseDatasetURL(datasetURL)
	if err != nil {
		return nil, err
	}
	client, err := bigquery.NewClient(ctx, ref.Project, option.WithCredentialsJSON(credsJSON))
	if err != nil {
		return nil, fmt.Errorf("warehouse: bigquery client: %w", err)
	}
	return &BQStore{client: client, ref: ref}, nil
}

// NewBigQueryFromEnv builds a Store using Application Default Credentials and
// GCLOUD_PROJECT + BIGQUERY_DATASET env vars (used by the ADK launcher CLI).
func NewBigQueryFromEnv(ctx context.Context) (Store, error) {
	project := os.Getenv("GCLOUD_PROJECT")
	dataset := os.Getenv("BIGQUERY_DATASET")
	if project == "" || dataset == "" {
		return nil, fmt.Errorf("warehouse: GCLOUD_PROJECT and BIGQUERY_DATASET must be set")
	}
	client, err := bigquery.NewClient(ctx, project)
	if err != nil {
		return nil, err
	}
	return &BQStore{client: client, ref: DatasetRef{Project: project, Dataset: dataset}}, nil
}

func (b *BQStore) table(name string) string {
	return fmt.Sprintf("`%s.%s.%s`", b.ref.Project, b.ref.Dataset, name)
}

func (b *BQStore) GetProducts(ctx context.Context) ([]models.Product, error) {
	q := b.client.Query(`SELECT product_id, name, category, supplier_id, unit_price, stock_level, reorder_pt, IFNULL(shelf_life_days, 0) AS shelf_life_days FROM ` + b.table("products"))
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Product
	for {
		var r struct {
			ProductID     string  `bigquery:"product_id"`
			Name          string  `bigquery:"name"`
			Category      string  `bigquery:"category"`
			SupplierID    string  `bigquery:"supplier_id"`
			UnitPrice     float64 `bigquery:"unit_price"`
			StockLevel    int     `bigquery:"stock_level"`
			ReorderPt     int     `bigquery:"reorder_pt"`
			ShelfLifeDays int     `bigquery:"shelf_life_days"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.Product{
			ID: r.ProductID, Name: r.Name, Category: r.Category, SupplierID: r.SupplierID,
			UnitPrice: r.UnitPrice, StockLevel: r.StockLevel, ReorderPt: r.ReorderPt, ShelfLifeDays: r.ShelfLifeDays,
		})
	}
	return out, nil
}

func (b *BQStore) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	q := b.client.Query(`SELECT product_id, quantity, total, CAST(date AS STRING) AS date FROM ` + b.table("sales") +
		` WHERE product_id = @pid AND date >= DATE_SUB(CURRENT_DATE(), INTERVAL 90 DAY)`)
	q.Parameters = []bigquery.QueryParameter{{Name: "pid", Value: productID}}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Sale
	for {
		var r struct {
			ProductID string  `bigquery:"product_id"`
			Quantity  int     `bigquery:"quantity"`
			Total     float64 `bigquery:"total"`
			Date      string  `bigquery:"date"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.Sale{ProductID: r.ProductID, Quantity: r.Quantity, Total: r.Total, Date: r.Date})
	}
	return out, nil
}

func (b *BQStore) GetSuppliers(ctx context.Context) ([]models.Supplier, error) {
	q := b.client.Query(`SELECT supplier_id, name, reliability, lead_time_days, avg_unit_price FROM ` + b.table("suppliers"))
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Supplier
	for {
		var r struct {
			SupplierID   string  `bigquery:"supplier_id"`
			Name         string  `bigquery:"name"`
			Reliability  float64 `bigquery:"reliability"`
			LeadTimeDays int     `bigquery:"lead_time_days"`
			AvgUnitPrice float64 `bigquery:"avg_unit_price"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.Supplier{ID: r.SupplierID, Name: r.Name, Reliability: r.Reliability, LeadTimeDays: r.LeadTimeDays, AvgUnitPrice: r.AvgUnitPrice})
	}
	return out, nil
}

func (b *BQStore) GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error) {
	q := b.client.Query(`SELECT CAST(date AS STRING) AS date, SUM(quantity) AS quantity FROM ` + b.table("sales") +
		` WHERE date >= DATE_SUB(CURRENT_DATE(), INTERVAL @days DAY) GROUP BY date ORDER BY date`)
	q.Parameters = []bigquery.QueryParameter{{Name: "days", Value: days}}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.DailySales
	for {
		var r struct {
			Date     string `bigquery:"date"`
			Quantity int    `bigquery:"quantity"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.DailySales{Date: r.Date, Quantity: r.Quantity})
	}
	return out, nil
}

func (b *BQStore) GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error) {
	q := b.client.Query(`SELECT s.product_id AS product_id, ANY_VALUE(p.name) AS name, SUM(s.quantity) AS units FROM ` +
		b.table("sales") + ` s JOIN ` + b.table("products") + ` p USING (product_id) ` +
		`GROUP BY s.product_id ORDER BY units DESC LIMIT @lim`)
	q.Parameters = []bigquery.QueryParameter{{Name: "lim", Value: limit}}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.TopSeller
	for {
		var r struct {
			ProductID string `bigquery:"product_id"`
			Name      string `bigquery:"name"`
			Units     int    `bigquery:"units"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.TopSeller{ProductID: r.ProductID, Name: r.Name, Units: r.Units})
	}
	return out, nil
}
