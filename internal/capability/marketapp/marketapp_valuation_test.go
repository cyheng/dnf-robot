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

func TestValueModelPriceIsMonotonic(t *testing.T) {
	app := testApp(t)
	app.cfg.Restock.ValueModelEnabled = true
	app.cfg.Restock.ValueCategoryRecognition = defaultValueCategoryRecognition()
	app.cfg.Restock.ValueCurveSpan = 6
	low := app.valueModelCenterPrice(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk", Rarity: 0})
	high := app.valueModelCenterPrice(catalogItem{Kind: "stackable", Path: "stackable/monstercard/a.stk", Rarity: 5})
	if low <= 0 || high <= low {
		t.Fatalf("value curve is not monotonic: low=%v high=%v", low, high)
	}
}
