package marketapp

import (
	"fmt"
	"math"
	"sort"
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
	valueCategoryOther      = "other"
)

func defaultValueCategoryRecognition() map[string]float64 {
	return map[string]float64{
		valueCategoryEquipment: 45, valueCategoryTitle: 95, valueCategoryCard: 85,
		valueCategoryCreature: 100, valueCategoryArtifact: 90, valueCategoryBead: 80,
		valueCategoryRecipe: 40, valueCategoryMaterial: 10, valueCategoryConsumable: 20,
		valueCategoryOther: 20,
	}
}

func encodeValueCategoryRecognition(values map[string]float64) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"|"+formatFloat(values[key]))
	}
	return strings.Join(parts, ";")
}

func decodeValueCategoryRecognition(value string) (map[string]float64, error) {
	result := defaultValueCategoryRecognition()
	value = strings.TrimSpace(value)
	if value == "" {
		return result, nil
	}
	for _, entry := range strings.Split(value, ";") {
		parts := strings.SplitN(strings.TrimSpace(entry), "|", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("invalid value category recognition %q", entry)
		}
		score, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || score < 0 || score > 100 {
			return nil, fmt.Errorf("invalid value category score %q", entry)
		}
		result[strings.ToLower(strings.TrimSpace(parts[0]))] = score
	}
	return result, nil
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
	case strings.HasPrefix(path, "stackable/monstercard/") || strings.Contains(slot, "material expert job"):
		return valueCategoryCard
	case strings.HasPrefix(path, "stackable/professional/bead/") || strings.Contains(slot, "enchant waste"):
		return valueCategoryBead
	case strings.HasPrefix(path, "stackable/recipe/") || strings.Contains(slot, "recipe"):
		return valueCategoryRecipe
	case strings.HasPrefix(path, "stackable/material/") || strings.Contains(slot, "material"):
		return valueCategoryMaterial
	case strings.HasPrefix(path, "stackable/professional/") || strings.Contains(slot, "potion") || strings.Contains(slot, "consum"):
		return valueCategoryConsumable
	default:
		return valueCategoryOther
	}
}

type valueScoreDetail struct {
	Category      string  `json:"category"`
	Score         float64 `json:"score"`
	CategoryScore float64 `json:"category_score"`
	RarityScore   float64 `json:"rarity_score"`
	LevelScore    float64 `json:"level_score"`
	PVFScore      float64 `json:"pvf_score"`
}

func (a *App) valueScore(item catalogItem) valueScoreDetail {
	return valueScoreWithConfig(item, a.configSnapshot().Restock)
}

func valueScoreWithConfig(item catalogItem, cfg RestockCfg) valueScoreDetail {
	recognition := cfg.ValueCategoryRecognition[valueCategory(item)]
	if recognition <= 0 {
		recognition = cfg.ValueCategoryRecognition[valueCategoryOther]
	}
	if recognition <= 0 {
		recognition = 20
	}
	rarity := float64(item.Rarity)
	if rarity < 0 {
		rarity = 0
	}
	if rarity > 5 {
		rarity = 5
	}
	level := float64(item.Level)
	if level < 0 {
		level = 0
	}
	if level > 70 {
		level = 70
	}
	raw := item.Price
	if raw <= 0 {
		raw = item.Value
	}
	pvfScore := 0.0
	if raw > 10 {
		pvfScore = math.Log10(float64(raw)) / 6
		if pvfScore > 1 {
			pvfScore = 1
		}
	}
	wc, wr, wl, wp := cfg.ValueCategoryWeight, cfg.ValueRarityWeight, cfg.ValueLevelWeight, cfg.ValuePVFWeight
	if wc < 0 {
		wc = 0
	}
	if wr < 0 {
		wr = 0
	}
	if wl < 0 {
		wl = 0
	}
	if wp < 0 {
		wp = 0
	}
	totalWeight := wc + wr + wl + wp
	if totalWeight <= 0 {
		wc, wr, wl, wp, totalWeight = .5, .25, .15, .1, 1
	}
	score := (wc*(recognition/100) + wr*(rarity/5) + wl*(level/70) + wp*pvfScore) / totalWeight
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return valueScoreDetail{Category: valueCategory(item), Score: score, CategoryScore: recognition, RarityScore: rarity / 5, LevelScore: level / 70, PVFScore: pvfScore}
}

func (a *App) valueModelCenterPrice(item catalogItem) float64 {
	return valueModelCenterPriceWithConfig(item, a.configSnapshot().Restock)
}

func valueModelCenterPriceWithConfig(item catalogItem, cfg RestockCfg) float64 {
	base := float64(cfg.ValueBasePrice)
	if base <= 0 {
		base = 1000
	}
	span := cfg.ValueCurveSpan
	if span <= 0 {
		span = 6
	}
	if span > 12 {
		span = 12
	}
	return base * math.Pow(10, valueScoreWithConfig(item, cfg).Score*span)
}
