package marketapp

import (
	"os"
	"testing"
)

func TestCustomPriceRangeOverridesFullEquipmentFormula(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.CustomPriceEnabled = true
	mustWriteJSON(t, appPaths(app).MarketPrices(), customPriceRangeFile{Version: 1, Items: []customPriceRange{{ItemID: 31056, MinPrice: 700000, MaxPrice: 700000, Enabled: true}}})
	app.refreshCustomPriceRanges()

	price := app.auctionUnitPriceFor(catalogItem{ItemID: 31056, Kind: "equipment"}, 8, 13)
	if price != 700000 {
		t.Fatalf("custom price=%d want 700000", price)
	}
	low, high := app.auctionPriceBounds(catalogItem{ItemID: 31056, Kind: "equipment", Price: 1000})
	if low != 700000 || high != 700000 {
		t.Fatalf("custom bounds=%d..%d", low, high)
	}
}

func TestCustomEquipmentPriceRangeHonorsFinalMaximum(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.CustomPriceEnabled = true
	app.cfg.Restock.EquipmentFinalMaxPrice = 200000000
	mustWriteJSON(t, appPaths(app).MarketPrices(), customPriceRangeFile{Version: 1, Items: []customPriceRange{{ItemID: 31056, MinPrice: 300000000, MaxPrice: 400000000, Enabled: true}}})
	app.refreshCustomPriceRanges()

	price := app.auctionUnitPriceFor(catalogItem{ItemID: 31056, Kind: "equipment"}, 1, 0)
	if price != 200000000 {
		t.Fatalf("custom price=%d want final maximum 200000000", price)
	}
	low, high := app.auctionPriceBounds(catalogItem{ItemID: 31056, Kind: "equipment"})
	if low != 200000000 || high != 200000000 {
		t.Fatalf("custom bounds=%d..%d want 200000000..200000000", low, high)
	}
}

func TestEquipmentFormulaBoundsIncludeUpgradeAndRandomRate(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.RandLow = 0.9
	app.cfg.Restock.RandHigh = 1.1
	app.cfg.Restock.UpgradeMin = 7
	app.cfg.Restock.UpgradeMax = 13
	app.cfg.Restock.UpgradePriceRate = 0.08
	setFixedValueModelBase(&app.cfg.Restock, valueCategoryEquipment, 1000)

	low, high := app.auctionPriceBounds(catalogItem{ItemID: 31056, Kind: "equipment", Slot: "weapon", Price: 1000})
	if low != 1404 || high != 6071 {
		t.Fatalf("formula bounds=%d..%d want 1404..6071", low, high)
	}
}

func TestUpgradePriceFactorIsLinearThroughTenAndAddsRiskPremiumAboveTen(t *testing.T) {
	tests := []struct {
		upgrade int
		want    float64
	}{
		{upgrade: 7, want: 1.56},
		{upgrade: 10, want: 1.80},
		{upgrade: 11, want: 1.96},
		{upgrade: 12, want: 2.28},
		{upgrade: 13, want: 2.76},
	}
	for _, tt := range tests {
		if got := auctionUpgradePriceFactor(tt.upgrade, 0.08); got < tt.want-1e-9 || got > tt.want+1e-9 {
			t.Errorf("upgrade +%d factor=%v want %v", tt.upgrade, got, tt.want)
		}
	}
}

func TestEquipmentMultiplierAppliesOnlyToEquipment(t *testing.T) {
	app := testApp(t)
	setFixedValueModelBase(&app.cfg.Restock, valueCategoryEquipment, 1000)
	setFixedValueModelBase(&app.cfg.Restock, valueCategoryOther, 1000)
	app.cfg.Restock.UpgradePriceRate = 0
	app.cfg.Restock.RandLow, app.cfg.Restock.RandHigh = 1, 1
	if got := app.auctionUnitPriceFor(catalogItem{Kind: "equipment"}, 1.5, 0); got != 1500 {
		t.Fatalf("equipment multiplier price=%d want 1500", got)
	}
	if got := app.auctionUnitPriceFor(catalogItem{Kind: "stackable"}, 9, 0); got != 1000 {
		t.Fatalf("stackable used equipment multiplier: price=%d want 1000", got)
	}
}

func TestEquipmentFormulaBoundsExcludeUpgradeForUnsupportedSlot(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.RandLow = 1
	app.cfg.Restock.RandHigh = 1
	app.cfg.Restock.UpgradeMin = 13
	app.cfg.Restock.UpgradeMax = 13
	app.cfg.Restock.UpgradePriceRate = 0.08
	setFixedValueModelBase(&app.cfg.Restock, valueCategoryEquipment, 1000)

	low, high := app.auctionPriceBounds(catalogItem{Kind: "equipment", Slot: "unknown", Price: 1000})
	if low != 1000 || high != 2000 {
		t.Fatalf("unsupported slot bounds=%d..%d want 1000..2000", low, high)
	}
}

func TestValueModelAppliesToEquipmentAndStackableItems(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.CategoryPriceRules[valueCategoryEquipment] = PriceRule{MinPrice: 1000, MaxPrice: 100000, RarityWeight: 1}
	app.cfg.Restock.CategoryPriceRules[valueCategoryOther] = PriceRule{MinPrice: 1000, MaxPrice: 100000, RarityWeight: 1}
	app.cfg.Restock.UpgradePriceRate = 0
	app.cfg.Restock.RandLow = 1
	app.cfg.Restock.RandHigh = 1

	equipment := catalogItem{Kind: "equipment", Rarity: 4}
	if got := app.auctionUnitPriceFor(equipment, 1, 0); got != 39810 {
		t.Fatalf("equipment price=%d want 39810", got)
	}
	stackable := catalogItem{Kind: "stackable", Rarity: 2}
	if got := app.auctionUnitPriceFor(stackable, 99, 31); got != 6309 {
		t.Fatalf("stackable price=%d want 6309", got)
	}
}

func TestValueModelIsIncludedInCollectorBounds(t *testing.T) {
	app := testApp(t)
	setFixedValueModelBase(&app.cfg.Restock, valueCategoryEquipment, 1000)
	app.cfg.Restock.UpgradeMin = 0
	app.cfg.Restock.UpgradeMax = 0
	app.cfg.Restock.RandLow = 1
	app.cfg.Restock.RandHigh = 1

	low, high := app.auctionPriceBounds(catalogItem{Kind: "equipment", Slot: "coat", Level: 5, Rarity: 2, Price: 1000})
	if low != 1000 || high != 2000 {
		t.Fatalf("value-model bounds=%d..%d want 1000..2000", low, high)
	}
}

func TestLegacySellPricesNeverGrantPurchasePermission(t *testing.T) {
	app := testApp(t)
	app.cfg.Collector.PriceRangeEnabled = true
	app.cfg.Collector.InRangeProbability = 1
	app.cfg.Collector.OutRangeProbability = 0
	app.cfg.Restock.CustomPriceEnabled = true
	mustWriteJSON(t, appPaths(app).MarketPrices(), customPriceRangeFile{Version: 1, Items: []customPriceRange{{ItemID: 3037, MinPrice: 80, MaxPrice: 120, Enabled: true}}})
	app.repository = &clearStockRepository{collectRows: map[string][]collectRow{
		app.cfg.AuctionDB: {
			{Market: marketNameAuction, AuctionID: 1, ItemID: 3037, Count: 10, StartPrice: -1, InstantPrice: 1000},
			{Market: marketNameAuction, AuctionID: 2, ItemID: 3037, Count: 10, StartPrice: -1, InstantPrice: 3000},
		},
	}}

	result, err := app.CollectPlan(CollectRequest{Market: marketNameAuction})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 0 {
		t.Fatalf("旧版卖价不能授予收购权限：%#v", result.Actions)
	}
}

func TestInvalidCustomPriceFileFallsBackToFormula(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.CustomPriceEnabled = true
	mustWriteText(t, appPaths(app).MarketPrices(), "{broken")
	app.refreshCustomPriceRanges()

	if app.priceRangeStatus.Error == "" {
		t.Fatal("invalid custom price file did not report an error")
	}
	setFixedValueModelBase(&app.cfg.Restock, valueCategoryOther, 100)
	app.cfg.Restock.RandLow = 1
	app.cfg.Restock.RandHigh = 1
	price := app.auctionUnitPriceFor(catalogItem{ItemID: 3037, Kind: "stackable"}, 1, 0)
	if price != 100 {
		t.Fatalf("value-model fallback price=%d want 100", price)
	}
}

func TestReloadCustomPriceRangeFileRebuildsDefaultWhenDeleted(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.CustomPriceEnabled = true
	doc := defaultCustomPriceRangeDocument()
	doc.Items = []customPriceRange{
		{ItemID: 4000, Name: "材料", SellEnabled: true, SellPrice: 200, TargetQuantity: 1000, StackSize: 100, BuyEnabled: true, BuyMaxPrice: 100, BuyDailyQuantityLimit: 1000, UpgradePolicy: "ignore"},
	}
	mustWriteJSON(t, appPaths(app).MarketPrices(), doc)
	app.refreshCustomPriceRanges()
	if _, status := app.businessSnapshot(); status.Error != "" {
		t.Fatalf("初始加载失败：%s", status.Error)
	}

	// 删除文件，模拟用户在服务器上删 market_item_price_ranges.json。
	if err := os.Remove(appPaths(app).MarketPrices()); err != nil {
		t.Fatal(err)
	}
	// 文件监听器回调应重建默认清单，而不是报错挂起经营。
	if err := app.reloadCustomPriceRangeFile(appPaths(app).MarketPrices()); err != nil {
		t.Fatalf("删除后重载不应报错：%v", err)
	}
	if _, err := os.Stat(appPaths(app).MarketPrices()); err != nil {
		t.Fatalf("默认清单未被重建：%v", err)
	}
	ranges, status := app.businessSnapshot()
	if status.Error != "" {
		t.Fatalf("重载后状态错误：%s", status.Error)
	}
	if status.Version != 2 || len(ranges) == 0 {
		t.Fatalf("重载后未恢复默认清单：version=%d items=%d", status.Version, len(ranges))
	}
}

func setFixedValueModelBase(cfg *RestockCfg, category string, base int32) {
	rule := PriceRule{MinPrice: base, MaxPrice: base, RarityWeight: 1}
	if category == valueCategoryEquipment {
		cfg.CategoryPriceRules[category] = rule
		cfg.EquipmentFinalMaxPrice = maxInt32
		return
	}
	cfg.CategoryPriceRules[category] = rule
}
