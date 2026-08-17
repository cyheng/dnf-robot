package marketapp

import "testing"

func TestValueCategoryUsesPathBeforeSlotFallback(t *testing.T) {
	cases := []struct {
		name string
		item catalogItem
		want string
	}{
		{"title", catalogItem{Kind: "equipment", ItemType: 2, Slot: "title name"}, valueCategoryTitle},
		{"card", catalogItem{Kind: "stackable", Path: "stackable/monstercard/example.stk"}, valueCategoryCard},
		{"bead", catalogItem{Kind: "stackable", Path: "stackable/professional/bead/example.stk"}, valueCategoryBead},
		{"recipe", catalogItem{Kind: "stackable", Path: "stackable/recipe/example.stk"}, valueCategoryRecipe},
		{"material", catalogItem{Kind: "stackable", Path: "stackable/professional/material/example.stk", Slot: "material expert job"}, valueCategoryMaterial},
		{"potion", catalogItem{Kind: "stackable", Path: "stackable/professional/potion/example.stk", Slot: "material expert job"}, valueCategoryConsumable},
		{"puppet", catalogItem{Kind: "stackable", Path: "stackable/professional/puppet/example.stk"}, valueCategoryPuppet},
		{"equipment", catalogItem{Kind: "equipment", ItemType: 1, Slot: "weapon"}, valueCategoryEquipment},
	}
	for _, tt := range cases {
		if got := valueCategory(tt.item); got != tt.want {
			t.Errorf("%s category=%q want %q", tt.name, got, tt.want)
		}
	}
}

func TestDefaultCategoryPriceRulesAreComplete(t *testing.T) {
	cfg := DefaultConfig().Restock
	if len(cfg.CategoryPriceRules) != len(categoryPriceRuleKeys()) {
		t.Fatalf("category rules=%d want %d", len(cfg.CategoryPriceRules), len(categoryPriceRuleKeys()))
	}
	potion := cfg.CategoryPriceRules[valueCategoryConsumable]
	if potion.MinPrice != 200 || potion.MaxPrice != 30000 {
		t.Fatalf("potion range=%+v", potion)
	}
	if rule := cfg.CategoryPriceRules[valueCategoryEquipment]; rule.MinPrice != 10000 || rule.MaxPrice != 50000000 {
		t.Fatalf("equipment range=%+v", rule)
	}
}

func TestPriceRuleReweightsWhenPVFValueIsMissing(t *testing.T) {
	rule := PriceRule{MinPrice: 100, MaxPrice: 100000, RarityWeight: .4, LevelWeight: .2, PVFWeight: .4}
	withoutPVF := priceRuleScore(catalogItem{Rarity: 5, Level: 70, Price: 1}, rule)
	withPVF := priceRuleScore(catalogItem{Rarity: 5, Level: 70, Price: 1000000}, rule)
	if withoutPVF != 1 || withPVF != 1 {
		t.Fatalf("missing signal was not reweighted: without=%v with=%v", withoutPVF, withPVF)
	}
}

func TestCategoryPriceStaysInsideConfiguredRange(t *testing.T) {
	cfg := DefaultConfig().Restock
	for _, category := range categoryPriceRuleKeys() {
		rule := cfg.CategoryPriceRules[category]
		low := priceFromRule(catalogItem{}, rule)
		high := priceFromRule(catalogItem{Level: 70, Rarity: 5, Price: 1000000}, rule)
		if low < float64(rule.MinPrice) || high > float64(rule.MaxPrice) || high < low {
			t.Fatalf("%s prices=%v..%v rule=%+v", category, low, high, rule)
		}
	}
}

func TestCategoryPriceIsMonotonicWithinRange(t *testing.T) {
	rule := defaultCategoryPriceRules()[valueCategoryCard]
	low := priceFromRule(catalogItem{Rarity: 0}, rule)
	high := priceFromRule(catalogItem{Rarity: 5, Level: 70, Price: 1000000}, rule)
	if low <= 0 || high <= low {
		t.Fatalf("category curve is not monotonic: low=%v high=%v", low, high)
	}
}
