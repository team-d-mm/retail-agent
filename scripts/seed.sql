-- Dataset
CREATE SCHEMA IF NOT EXISTS `PROJECT.retail`;

-- Products (note: milk/bread are perishable with short shelf_life_days)
CREATE OR REPLACE TABLE `PROJECT.retail.products` (
  product_id STRING, name STRING, category STRING, supplier_id STRING,
  unit_price FLOAT64, stock_level INT64, reorder_pt INT64, shelf_life_days INT64
);
INSERT INTO `PROJECT.retail.products` VALUES
  ('P1','Milk','dairy','S1',1.20,5,20,4),
  ('P2','Bread','bakery','S2',0.90,8,25,3),
  ('P3','Rice 5kg','staple','S1',6.50,100,20,0),
  ('P4','Eggs (dozen)','dairy','S1',2.10,10,30,14),
  ('P5','Bananas','produce','S2',0.40,12,40,5),
  ('P6','Cooking Oil','staple','S3',3.20,50,15,0);

-- Suppliers
CREATE OR REPLACE TABLE `PROJECT.retail.suppliers` (
  supplier_id STRING, name STRING, reliability FLOAT64, lead_time_days INT64, avg_unit_price FLOAT64
);
INSERT INTO `PROJECT.retail.suppliers` VALUES
  ('S1','FreshFarm',0.95,3,3.00),
  ('S2','DailyGoods',0.88,2,0.70),
  ('S3','BulkSupply',0.92,5,3.10);

-- Sales: ~90 days of history. Generate steady daily sales per product.
CREATE OR REPLACE TABLE `PROJECT.retail.sales` AS
SELECT
  GENERATE_UUID() AS sale_id,
  p.product_id,
  CAST(ROUND(p.daily * (0.8 + RAND()*0.4)) AS INT64) AS quantity,
  ROUND(p.daily * p.price, 2) AS total,
  d AS date
FROM UNNEST(GENERATE_DATE_ARRAY(DATE_SUB(CURRENT_DATE(), INTERVAL 89 DAY), CURRENT_DATE())) AS d
CROSS JOIN (
  SELECT 'P1' AS product_id, 10 AS daily, 1.20 AS price UNION ALL
  SELECT 'P2', 9, 0.90 UNION ALL
  SELECT 'P3', 3, 6.50 UNION ALL
  SELECT 'P4', 6, 2.10 UNION ALL
  SELECT 'P5', 15, 0.40 UNION ALL
  SELECT 'P6', 4, 3.20
) AS p;
