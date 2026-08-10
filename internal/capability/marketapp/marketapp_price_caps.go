package marketapp

import (
	"fmt"
	"os"

	foundationconfig "robot/internal/foundation/config"
	"robot/internal/foundation/layout"
	"robot/internal/shared"
)

const (
	equipmentPriceProtectionOff      = "off"
	equipmentPriceProtectionStrict   = "strict"
	equipmentPriceProtectionStandard = "standard"
	equipmentPriceProtectionRelaxed  = "relaxed"
)

func validEquipmentPriceProtection(value string) bool {
	switch value {
	case equipmentPriceProtectionOff, equipmentPriceProtectionStrict, equipmentPriceProtectionStandard, equipmentPriceProtectionRelaxed:
		return true
	default:
		return false
	}
}

func (a *App) refreshEquipmentPriceCaps() error {
	path := layout.New(a.configDir).PVFEquipmentPriceCaps()
	data, err := os.ReadFile(path)
	if err != nil {
		a.setEquipmentPriceCaps(nil)
		return err
	}
	var doc shared.EquipmentLevelPriceCapsDocument
	if err := foundationconfig.DecodeJSONBytes(data, &doc); err != nil {
		a.setEquipmentPriceCaps(nil)
		return err
	}
	if doc.Version != 1 {
		a.setEquipmentPriceCaps(nil)
		return fmt.Errorf("unsupported equipment level price caps version %d", doc.Version)
	}
	caps := make(map[int]shared.EquipmentLevelPriceCap, len(doc.Levels))
	for index, row := range doc.Levels {
		if row.Level < 0 || row.Level > marketConfigMaxLevel || row.Samples < 4 || row.StrictCap <= 0 || row.StandardCap < row.StrictCap || row.RelaxedCap < row.StandardCap || row.RelaxedCap > int64(maxInt32) {
			a.setEquipmentPriceCaps(nil)
			return fmt.Errorf("invalid equipment level price cap at index %d", index)
		}
		if _, exists := caps[row.Level]; exists {
			a.setEquipmentPriceCaps(nil)
			return fmt.Errorf("duplicate equipment level price cap for level %d", row.Level)
		}
		caps[row.Level] = row
	}
	a.setEquipmentPriceCaps(caps)
	return nil
}

func (a *App) setEquipmentPriceCaps(caps map[int]shared.EquipmentLevelPriceCap) {
	if caps == nil {
		caps = map[int]shared.EquipmentLevelPriceCap{}
	}
	a.stateMu.Lock()
	a.equipmentPriceCaps = caps
	a.stateMu.Unlock()
}

func (a *App) protectedEquipmentBasePrice(item catalogItem, base int32) int32 {
	if base <= 0 || item.Kind != "equipment" || specialAuctionKind(item) != "" {
		return base
	}
	policy := a.configSnapshot().Restock.EquipmentPriceProtection
	if policy == equipmentPriceProtectionOff {
		return base
	}
	a.stateMu.RLock()
	row, ok := a.equipmentPriceCaps[item.Level]
	if !ok {
		bestDistance := int(^uint(0) >> 1)
		for level, candidate := range a.equipmentPriceCaps {
			distance := level - item.Level
			if distance < 0 {
				distance = -distance
			}
			if distance < bestDistance {
				bestDistance, row, ok = distance, candidate, true
			}
		}
	}
	a.stateMu.RUnlock()
	if !ok {
		return base
	}
	cap := row.StandardCap
	switch policy {
	case equipmentPriceProtectionStrict:
		cap = row.StrictCap
	case equipmentPriceProtectionRelaxed:
		cap = row.RelaxedCap
	}
	if cap > 0 && int64(base) > cap {
		return int32(cap)
	}
	return base
}
