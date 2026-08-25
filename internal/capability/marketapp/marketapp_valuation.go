package marketapp

import (
	"math"
	"strconv"
	"strings"
)

const (
	valueCategoryEquipment  = "equipment"
	valueCategoryTitle      = "title"
	valueCategoryCard       = "card"
	valueCategoryCreature   = "creature"
	valueCategoryArtifact   = "artifact"
	valueCategoryBead       = "bead"
	valueCategoryRecipe     = "recipe"
	valueCategoryMaterial   = "material"
	valueCategoryConsumable = "consumable"
	valueCategoryPuppet     = "puppet"
	valueCategoryOther      = "other"
)

func categoryPriceRuleKeys() []string {
	return []string{
		valueCategoryEquipment, valueCategoryTitle, valueCategoryCard, valueCategoryCreature, valueCategoryArtifact,
		valueCategoryBead, valueCategoryRecipe, valueCategoryMaterial, valueCategoryConsumable,
		valueCategoryPuppet, valueCategoryOther,
	}
}

func validCategoryPriceRule(value string) bool {
	for _, key := range categoryPriceRuleKeys() {
		if value == key {
			return true
		}
	}
	return false
}

func defaultCategoryPriceRules() map[string]PriceRule {
	rule := func(min, max int32, rarity, level, pvf float64) PriceRule {
		return PriceRule{MinPrice: min, MaxPrice: max, RarityWeight: rarity, LevelWeight: level, PVFWeight: pvf}
	}
	rules := map[string]PriceRule{
		valueCategoryEquipment:  rule(10000, 50000000, .4, .35, .25),
		valueCategoryTitle:      rule(100000, 10000000, .4, .2, .4),
		valueCategoryCard:       rule(20000, 20000000, .4, .2, .4),
		valueCategoryCreature:   rule(100000, 20000000, .4, .2, .4),
		valueCategoryArtifact:   rule(10000, 5000000, .4, .2, .4),
		valueCategoryBead:       rule(10000, 5000000, .4, .2, .4),
		valueCategoryRecipe:     rule(1000, 500000, .4, .2, .4),
		valueCategoryMaterial:   rule(100, 50000, .4, .2, .4),
		valueCategoryConsumable: rule(200, 30000, .4, .2, .4),
		valueCategoryPuppet:     rule(5000, 200000, .4, .2, .4),
		valueCategoryOther:      rule(100, 100000, .4, .2, .4),
	}
	equipment := rules[valueCategoryEquipment]
	equipment.RarityScoreCurve = defaultEquipmentRarityScoreCurve
	rules[valueCategoryEquipment] = equipment
	return rules
}

func valueCategory(item catalogItem) string {
	if item.Kind == "equipment" {
		switch specialAuctionKind(item) {
		case valueCategoryTitle, valueCategoryCreature:
			return specialAuctionKind(item)
		case "artifact red", "artifact blue", "artifact green":
			return valueCategoryArtifact
		default:
			return valueCategoryEquipment
		}
	}
	path := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(item.Path), "\\", "/"))
	slot := strings.ToLower(strings.TrimSpace(item.Slot))
	switch {
	case strings.HasPrefix(path, "stackable/professional/potion/"):
		return valueCategoryConsumable
	case strings.HasPrefix(path, "stackable/professional/puppet/") || strings.HasPrefix(path, "stackable/professional/common/") && strings.Contains(path, "doll"):
		return valueCategoryPuppet
	case strings.HasPrefix(path, "stackable/professional/material/"):
		return valueCategoryMaterial
	case strings.HasPrefix(path, "stackable/professional/bead/"):
		return valueCategoryBead
	case strings.HasPrefix(path, "stackable/monstercard/"):
		return valueCategoryCard
	case strings.HasPrefix(path, "stackable/recipe/"):
		return valueCategoryRecipe
	case strings.HasPrefix(path, "stackable/material/"):
		return valueCategoryMaterial
	case strings.Contains(slot, "enchant waste"):
		return valueCategoryBead
	case strings.Contains(slot, "recipe"):
		return valueCategoryRecipe
	case strings.Contains(slot, "material expert job"):
		return valueCategoryCard
	case strings.Contains(slot, "material"):
		return valueCategoryMaterial
	case strings.HasPrefix(path, "stackable/professional/") || strings.Contains(slot, "potion") || strings.Contains(slot, "consum"):
		return valueCategoryConsumable
	default:
		return valueCategoryOther
	}
}

func priceRuleScore(item catalogItem, rule PriceRule) float64 {
	rarity := rarityScore(item.Rarity, rule.RarityScoreCurve)
	level := math.Max(0, math.Min(70, float64(item.Level))) / 70
	raw := item.Price
	if raw <= 10 {
		raw = item.Value
	}
	pvf, pvfWeight := 0.0, math.Max(0, rule.PVFWeight)
	if raw > 10 {
		pvf = math.Min(1, math.Log10(float64(raw))/6)
	} else {
		pvfWeight = 0
	}
	rarityWeight := math.Max(0, rule.RarityWeight)
	levelWeight := math.Max(0, rule.LevelWeight)
	total := rarityWeight + levelWeight + pvfWeight
	if total <= 0 {
		return 0
	}
	return (rarity*rarityWeight + level*levelWeight + pvf*pvfWeight) / total
}

// rarityScore preserves the historical linear 0..5 score when no curve is configured.
func rarityScore(value int, curve string) float64 {
	if strings.TrimSpace(curve) == "" {
		return math.Max(0, math.Min(5, float64(value))) / 5
	}
	for _, part := range strings.Split(curve, ";") {
		part = strings.TrimSpace(part)
		if len(part) < 5 || part[0] != '(' || part[len(part)-1] != ')' {
			continue
		}
		fields := strings.Split(part[1:len(part)-1], ",")
		if len(fields) != 2 {
			continue
		}
		rarity, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil || rarity != value {
			continue
		}
		raw := strings.TrimSpace(fields[1])
		if strings.HasSuffix(raw, "%") {
			raw = strings.TrimSpace(strings.TrimSuffix(raw, "%"))
		}
		score, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0
		}
		if strings.Contains(strings.TrimSpace(fields[1]), "%") {
			score /= 100
		}
		return math.Max(0, math.Min(1, score))
	}
	return math.Max(0, math.Min(5, float64(value))) / 5
}

func priceFromRule(item catalogItem, rule PriceRule) float64 {
	low, high := float64(rule.MinPrice), float64(rule.MaxPrice)
	if high <= low {
		return math.Max(1, low)
	}
	return low * math.Pow(high/low, priceRuleScore(item, rule))
}

func configuredCenterPrice(item catalogItem, cfg RestockCfg) float64 {
	category := valueCategory(item)
	if rule, ok := cfg.CategoryPriceRules[category]; ok {
		return priceFromRule(item, rule)
	}
	return priceFromRule(item, cfg.CategoryPriceRules[valueCategoryOther])
}
