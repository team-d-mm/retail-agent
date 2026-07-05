-- Dataset
CREATE SCHEMA IF NOT EXISTS `saanay-genai-adk.retail`;

-- Products: ~30 realistic branded grocery items (name includes brand + size).
-- Demo mix:
--   * perishables below reorder point (short shelf_life_days) -> reordered AND spoilage-capped
--   * staples above reorder point -> no order
--   * two items (P29, P30) have NO sales -> "no recent sales, no order" path
CREATE OR REPLACE TABLE `saanay-genai-adk.retail.products` (
  product_id STRING, name STRING, category STRING, supplier_id STRING,
  unit_price FLOAT64, stock_level INT64, reorder_pt INT64, shelf_life_days INT64
);
INSERT INTO `saanay-genai-adk.retail.products` VALUES
  ('P1','Coca Cola 350ml','beverages','S3',0.60,30,60,0),
  ('P2','Pepsi 500ml','beverages','S3',0.80,45,50,0),
  ('P3','Nestle Pure Life Water 1.5L','beverages','S3',0.50,200,50,0),
  ('P4','Red Bull 250ml','beverages','S4',1.50,15,40,0),
  ('P5','100 Plus 325ml','beverages','S3',0.70,60,40,0),
  ('P6','Dutch Lady Milk 1L','dairy','S1',1.80,5,20,4),
  ('P7','Farmhouse Fresh Milk 200ml','dairy','S1',0.90,8,30,5),
  ('P8','Anchor Butter 227g','dairy','S1',3.50,40,20,60),
  ('P9','Nestle Yogurt 130g','dairy','S1',0.75,14,35,12),
  ('P10','Eggs Grade A (dozen)','dairy','S1',2.10,25,30,21),
  ('P11','Gardenia White Bread 400g','bakery','S2',2.50,6,25,3),
  ('P12','Butter Croissant 4pk','bakery','S2',3.00,4,20,3),
  ('P13','Wholemeal Bun 6pk','bakery','S2',2.20,18,22,4),
  ('P14','Lays Classic 160g','snacks','S4',2.30,12,30,180),
  ('P15','Oreo Original 133g','snacks','S4',1.60,22,30,180),
  ('P16','KitKat 4-Finger 35g','snacks','S4',0.90,40,50,300),
  ('P17','Pringles Original 107g','snacks','S4',2.80,16,25,240),
  ('P18','Mister Potato 75g','snacks','S4',1.10,55,40,180),
  ('P19','Bananas 1kg','produce','S2',1.20,10,40,5),
  ('P20','Tomatoes 500g','produce','S2',0.90,12,35,6),
  ('P21','Local Apples 1kg','produce','S2',2.40,30,30,14),
  ('P22','Potatoes 2kg','produce','S2',1.80,45,25,30),
  ('P23','Jasmine Rice 5kg','staple','S1',6.50,100,20,0),
  ('P24','Cooking Oil 2L','staple','S3',5.20,50,15,0),
  ('P25','Sugar 1kg','staple','S3',1.30,80,25,0),
  ('P26','Salt 500g','staple','S3',0.50,60,15,0),
  ('P27','Maggi Curry Noodles 5pk','staple','S3',2.00,20,40,365),
  ('P28','Milo 3-in-1 15pk','beverages','S3',4.50,35,30,300),
  ('P29','Dettol Handwash 250ml','household','S4',2.90,5,20,0),
  ('P30','Colgate Toothpaste 175g','household','S4',3.20,18,20,0);

-- Suppliers: realistic distributors with varied reliability + lead time.
CREATE OR REPLACE TABLE `saanay-genai-adk.retail.suppliers` (
  supplier_id STRING, name STRING, reliability FLOAT64, lead_time_days INT64, avg_unit_price FLOAT64
);
INSERT INTO `saanay-genai-adk.retail.suppliers` VALUES
  ('S1','FreshFarm Dairy Co',0.95,3,1.80),
  ('S2','Green Valley Produce',0.88,2,1.90),
  ('S3','MegaMart Distribution',0.92,5,1.60),
  ('S4','SnackWorld Supply',0.90,4,2.10);

-- Sales: ~90 days of daily transactions for P1-P28 (P29/P30 intentionally have
-- none). Per-product daily rate varies; RAND adds realistic day-to-day noise.
CREATE OR REPLACE TABLE `saanay-genai-adk.retail.sales` AS
SELECT
  GENERATE_UUID() AS sale_id,
  p.product_id,
  CAST(ROUND(p.daily * (0.8 + RAND() * 0.4)) AS INT64) AS quantity,
  ROUND(p.daily * p.price, 2) AS total,
  d AS date
FROM UNNEST(GENERATE_DATE_ARRAY(DATE_SUB(CURRENT_DATE(), INTERVAL 89 DAY), CURRENT_DATE())) AS d
CROSS JOIN (
  SELECT 'P1'  AS product_id, 25 AS daily, 0.60 AS price UNION ALL
  SELECT 'P2',  18, 0.80 UNION ALL
  SELECT 'P3',  30, 0.50 UNION ALL
  SELECT 'P4',  10, 1.50 UNION ALL
  SELECT 'P5',  14, 0.70 UNION ALL
  SELECT 'P6',  12, 1.80 UNION ALL
  SELECT 'P7',  18, 0.90 UNION ALL
  SELECT 'P8',   3, 3.50 UNION ALL
  SELECT 'P9',  15, 0.75 UNION ALL
  SELECT 'P10',  8, 2.10 UNION ALL
  SELECT 'P11', 14, 2.50 UNION ALL
  SELECT 'P12',  9, 3.00 UNION ALL
  SELECT 'P13',  7, 2.20 UNION ALL
  SELECT 'P14',  8, 2.30 UNION ALL
  SELECT 'P15',  9, 1.60 UNION ALL
  SELECT 'P16', 20, 0.90 UNION ALL
  SELECT 'P17',  6, 2.80 UNION ALL
  SELECT 'P18', 12, 1.10 UNION ALL
  SELECT 'P19', 20, 1.20 UNION ALL
  SELECT 'P20', 16, 0.90 UNION ALL
  SELECT 'P21', 10, 2.40 UNION ALL
  SELECT 'P22',  6, 1.80 UNION ALL
  SELECT 'P23',  3, 6.50 UNION ALL
  SELECT 'P24',  4, 5.20 UNION ALL
  SELECT 'P25',  5, 1.30 UNION ALL
  SELECT 'P26',  2, 0.50 UNION ALL
  SELECT 'P27', 12, 2.00 UNION ALL
  SELECT 'P28',  7, 4.50
) AS p;
