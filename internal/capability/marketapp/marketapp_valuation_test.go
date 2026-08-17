package marketapp

import "testing"

func TestValueCategoryUsesEconomicGroups(t *testing.T) {
	cases := []struct {
		name string
		item catalogItem
		want string
	}{
		{"title", catalogItem{Kind: "equipment", ItemType: 2, Slot: "title name"}, valueCategoryTitle},
		{"card", catalogItem{Kind: "stackable", Path: "stackable/monstercard/example.stk"}, valueCategoryCard},
		{"bead", catalogItem{Kind: "stackable", Path: "stackable/professional/bead/example.stk"}, valueCategoryBead},
		{"recipe", catalogItem{Kind: "stackable", Path: "stackable/recipe/example.stk"}, valueCategoryRecipe},
		{"material", catalogItem{Kind: "stackable", Path: "stackable/material/example.stk", Slot: "material"}, valueCategoryMaterial},
		{"equipment", catalogItem{Kind: "equipment", ItemType: 1, Slot: "weapon"}, valueCategoryEquipment},
	}
	for _, tt := range cases {
		if got := valueCategory(tt.item); got != tt.want {
			t.Errorf("%s category=%q want %q", tt.name, got, tt.want)
		}
	}
}

func TestDefaultValueModelUsesTunedVMDistribution(t *testing.T) {
	cfg := DefaultConfig().Restock
	if cfg.ValueCategoryWeight != .45 || cfg.ValueRarityWeight != .25 || cfg.ValueLevelWeight != .20 || cfg.ValuePVFWeight != .10 {
		t.Fatalf("unexpected default value weights: %+v", cfg)
	}
	if cfg.ValueCurveSpan != 6 || cfg.ValueBasePrice != 1000 {
		t.Fatalf("unexpected default value curve: %+v", cfg)
	}
}

func TestValueScoreTreatsTinyPVFPricesAsMissingSignal(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.ValueCategoryRecognition = defaultValueCategoryRecognition()
	app.cfg.Restock.ValueCategoryWeight = .5
	app.cfg.Restock.ValueRarityWeight = .25
	app.cfg.Restock.ValueLevelWeight = .15
	app.cfg.Restock.ValuePVFWeight = .1
	low := app.valueScore(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk", Price: 1, Rarity: 3})
	high := app.valueScore(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk", Price: 100000, Rarity: 3})
	if low.Score >= high.Score {
		t.Fatalf("valid PVF signal should increase score: low=%+v high=%+v", low, high)
	}
	if low.PVFScore != 0 {
		t.Fatalf("tiny PVF price should be ignored, got %v", low.PVFScore)
	}
}

func TestValueScoreUsesPVFValueWhenPriceIsTiny(t *testing.T) {
	app := testApp(t)
	detail := app.valueScore(catalogItem{Kind: "stackable", Price: 1, Value: 100000})
	if detail.PVFScore <= 0 {
		t.Fatalf("PVF value should replace a tiny price signal: %+v", detail)
	}
}

func TestValueScorePreservesZeroCategoryRecognition(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.ValueCategoryRecognition[valueCategoryCard] = 0
	detail := app.valueScore(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk"})
	if detail.CategoryScore != 0 {
		t.Fatalf("explicit zero category recognition was replaced: %+v", detail)
	}
}

func TestDecodeValueCategoryRecognitionRejectsUnknownCategory(t *testing.T) {
	if _, err := decodeValueCategoryRecognition("unknown|50"); err == nil {
		t.Fatal("unknown value category was accepted")
	}
}

func TestZeroValueWeightsProduceBasePrice(t *testing.T) {
	cfg := DefaultConfig().Restock
	cfg.ValueCategoryWeight, cfg.ValueRarityWeight, cfg.ValueLevelWeight, cfg.ValuePVFWeight = 0, 0, 0, 0
	if got := valueModelCenterPriceWithConfig(catalogItem{Kind: "equipment", Level: 70, Rarity: 5, Price: 1000000}, cfg); got != float64(cfg.ValueBasePrice) {
		t.Fatalf("zero weights price=%v want base %d", got, cfg.ValueBasePrice)
	}
}

func TestValueModelPriceIsMonotonic(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.ValueCategoryRecognition = defaultValueCategoryRecognition()
	app.cfg.Restock.ValueCurveSpan = 6
	low := app.valueModelCenterPrice(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk", Rarity: 0})
	high := app.valueModelCenterPrice(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk", Rarity: 5})
	if low <= 0 || high <= low {
		t.Fatalf("value curve is not monotonic: low=%v high=%v", low, high)
	}
}

func TestAuctionPlanPricingKeepsFullCatalogSignals(t *testing.T) {
	app := testApp(t)
	item := catalogItem{
		ItemID: 77, Kind: "stackable", Path: "stackable/monstercard/a.stk",
		Price: 1, Value: 100000, Rarity: 3,
	}
	_, planned := auctionPlanRow(restockRow{ItemID: 77, Kind: "stackable", SystemPrice: 1}, map[uint32]catalogItem{77: item})
	detail := app.valueScore(planned)
	if detail.Category != valueCategoryCard || detail.PVFScore <= 0 || planned.Value != 100000 {
		t.Fatalf("catalog pricing signals were lost: item=%+v score=%+v", planned, detail)
	}
}
