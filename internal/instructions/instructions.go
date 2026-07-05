package instructions

import _ "embed"

//go:embed retail_agent.md
var RetailAgent string

//go:embed inventory_agent.md
var InventoryAgent string

//go:embed demand_agent.md
var DemandAgent string

//go:embed supplier_agent.md
var SupplierAgent string
