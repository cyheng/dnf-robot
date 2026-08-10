package pvf

import (
	"testing"

	"robot/internal/shared"
)

func TestBuildEquipmentLevelPriceCapsRejectsExtremePVFPrice(t *testing.T) {
	prices := []int{100, 110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 5000}
	items := make([]shared.EquipmentCatalogItem, 0, len(prices))
	for index, price := range prices {
		items = append(items, shared.EquipmentCatalogItem{ID: index + 1, Level: 50, ItemType: 1, Slot: "weapon", Price: price})
	}
	doc := buildEquipmentLevelPriceCaps(items)
	if doc.Version != 1 || len(doc.Levels) != 51 {
		t.Fatalf("document version/levels = %d/%d, want 1/51", doc.Version, len(doc.Levels))
	}
	level := doc.Levels[50]
	if level.Samples != 12 || level.Q1 != 125 || level.Median != 155 || level.Q3 != 185 {
		t.Fatalf("level statistics = %+v", level)
	}
	if level.StrictCap != 275 || level.StandardCap != 365 || level.RelaxedCap != 545 {
		t.Fatalf("level caps = %+v, want 275/365/545", level)
	}
}

func TestBuildEquipmentLevelPriceCapsExcludesSpecialAndInvalidEquipment(t *testing.T) {
	items := []shared.EquipmentCatalogItem{
		{ID: 1, Level: 10, ItemType: 1, Slot: "weapon", Price: 100},
		{ID: 2, Level: 10, ItemType: 1, Slot: "coat", Price: 110},
		{ID: 3, Level: 10, ItemType: 1, Slot: "ring", Value: 120},
		{ID: 4, Level: 10, ItemType: 1, Slot: "shoes", Price: 130},
		{ID: 5, Level: 10, ItemType: 2, Slot: "title name", Price: 999999},
		{ID: 6, Level: 10, ItemType: 20, Slot: "avatar", Price: 999999},
		{ID: 7, Level: 10, ItemType: 1, Slot: "weapon", Price: 999999, ClientIncompatible: true},
	}
	doc := buildEquipmentLevelPriceCaps(items)
	level := doc.Levels[10]
	if level.Samples != 4 || level.Q3 != 125 {
		t.Fatalf("special equipment affected statistics: %+v", level)
	}
}
