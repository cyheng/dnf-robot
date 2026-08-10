package marketapp

import (
	"testing"

	"robot/internal/shared"
)

func TestEquipmentPriceProtectionPoliciesCapPVFBasePrice(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.LevelPriceRate = 0
	app.cfg.Restock.RarityPriceRate = 0
	app.cfg.Restock.UpgradePriceRate = 0
	app.cfg.Restock.EquipInflateMin = 1
	app.cfg.Restock.EquipInflateMax = 1
	app.equipmentPriceCaps = map[int]shared.EquipmentLevelPriceCap{
		50: {Level: 50, Samples: 12, StrictCap: 275, StandardCap: 365, RelaxedCap: 545},
	}
	item := catalogItem{Kind: "equipment", Slot: "weapon", Level: 50, Price: 5000}
	tests := []struct {
		policy string
		want   int32
	}{
		{policy: equipmentPriceProtectionOff, want: 5000},
		{policy: equipmentPriceProtectionStrict, want: 275},
		{policy: equipmentPriceProtectionStandard, want: 365},
		{policy: equipmentPriceProtectionRelaxed, want: 545},
	}
	for _, tt := range tests {
		app.cfg.Restock.EquipmentPriceProtection = tt.policy
		if got := app.auctionUnitPriceFor(item, item.Price, 1, 0); got != tt.want {
			t.Errorf("policy %s price=%d want %d", tt.policy, got, tt.want)
		}
	}
	app.cfg.Restock.EquipmentPriceProtection = equipmentPriceProtectionStandard
	low, high := app.auctionPriceBounds(item)
	if low != 365 || high != 365 {
		t.Fatalf("protected collector bounds=%d..%d want 365..365", low, high)
	}
}

func TestEquipmentPriceProtectionUsesNearestLevelAndSkipsSpecialEquipment(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.EquipmentPriceProtection = equipmentPriceProtectionStandard
	app.equipmentPriceCaps = map[int]shared.EquipmentLevelPriceCap{
		50: {Level: 50, Samples: 12, StrictCap: 275, StandardCap: 365, RelaxedCap: 545},
	}
	if got := app.protectedEquipmentBasePrice(catalogItem{Kind: "equipment", Slot: "coat", Level: 51}, 5000); got != 365 {
		t.Fatalf("nearest-level protected price=%d want 365", got)
	}
	if got := app.protectedEquipmentBasePrice(catalogItem{Kind: "equipment", ItemType: 2, Slot: "title name", Level: 50}, 5000); got != 5000 {
		t.Fatalf("special equipment price=%d want unchanged 5000", got)
	}
}

func TestRefreshEquipmentPriceCapsLoadsGeneratedPVFDocument(t *testing.T) {
	app := testApp(t)
	doc := shared.EquipmentLevelPriceCapsDocument{Version: 1, Levels: []shared.EquipmentLevelPriceCap{{
		Level: 50, Samples: 12, Q1: 125, Q3: 185, StrictCap: 275, StandardCap: 365, RelaxedCap: 545,
	}}}
	mustWriteJSON(t, appPaths(app).PVFEquipmentPriceCaps(), doc)
	if err := app.refreshEquipmentPriceCaps(); err != nil {
		t.Fatal(err)
	}
	if got := app.protectedEquipmentBasePrice(catalogItem{Kind: "equipment", Slot: "weapon", Level: 50}, 5000); got != 365 {
		t.Fatalf("loaded protected price=%d want 365", got)
	}
}

func TestCustomPriceRangeOverridesEquipmentPriceProtection(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.CustomPriceEnabled = true
	app.cfg.Restock.EquipmentPriceProtection = equipmentPriceProtectionStrict
	app.equipmentPriceCaps = map[int]shared.EquipmentLevelPriceCap{
		50: {Level: 50, Samples: 12, StrictCap: 275, StandardCap: 365, RelaxedCap: 545},
	}
	mustWriteJSON(t, appPaths(app).MarketPrices(), customPriceRangeFile{Version: 1, Items: []customPriceRange{{ItemID: 31056, MinPrice: 700000, MaxPrice: 700000, Enabled: true}}})
	app.refreshCustomPriceRanges()
	item := catalogItem{ItemID: 31056, Kind: "equipment", Slot: "weapon", Level: 50, Price: 5000}
	if got := app.auctionUnitPriceFor(item, item.Price, 1, 0); got != 700000 {
		t.Fatalf("custom protected price=%d want 700000", got)
	}
}
