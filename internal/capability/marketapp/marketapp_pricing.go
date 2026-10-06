package marketapp

import "math"

func (a *App) price(base int32) int32 {
	if base <= 0 {
		base = 1
	}
	cfg := a.configSnapshot()
	low, high := cfg.Restock.RandLow, cfg.Restock.RandHigh
	if low <= 0 || high <= 0 || low == high {
		return base
	}
	v := float64(base) * (low + a.randomFloat64()*(high-low))
	if v < 1 {
		return 1
	}
	if v > float64(maxInt32) {
		return maxInt32
	}
	return int32(v)
}

func (a *App) auctionUnitPriceFor(item catalogItem, equipmentMultiplier float64, upgrade int) int32 {
	rules, status := a.businessSnapshot()
	if rule, ok := rules[item.ItemID]; status.Version == 2 && ok && rule.SellEnabled {
		price, _ := a.businessPrices(rule, item, upgrade)
		return price
	}

	cfg := a.configSnapshot()
	if item.ItemID > 0 {
		if priceRange, ok := a.customPriceRange(item.ItemID); ok {
			price := a.randomPriceInRange(priceRange.MinPrice, priceRange.MaxPrice)
			if valueCategory(item) == valueCategoryEquipment {
				price = clampEquipmentFinalPrice(price, cfg.Restock)
			}
			return price
		}
	}
	price := configuredCenterPrice(item, cfg.Restock)
	category := valueCategory(item)
	if category == valueCategoryEquipment {
		if equipmentMultiplier <= 0 {
			equipmentMultiplier = 1
		}
		price *= equipmentMultiplier
		price *= auctionUpgradePriceFactor(upgrade, cfg.Restock.UpgradePriceRate)
	}
	low, high := cfg.Restock.RandLow, cfg.Restock.RandHigh
	if low > 0 && high > 0 && low != high {
		if high < low {
			high = low
		}
		price *= low + a.randomFloat64()*(high-low)
	}
	if category != valueCategoryEquipment {
		if rule, ok := cfg.Restock.CategoryPriceRules[category]; ok {
			if price < float64(rule.MinPrice) {
				price = float64(rule.MinPrice)
			}
			if price > float64(rule.MaxPrice) {
				price = float64(rule.MaxPrice)
			}
		}
	}
	if category == valueCategoryEquipment {
		if cfg.Restock.EquipmentFinalMaxPrice > 0 && price > float64(cfg.Restock.EquipmentFinalMaxPrice) {
			price = float64(cfg.Restock.EquipmentFinalMaxPrice)
		}
	}
	return boundedAuctionPrice(price)
}

func (a *App) randomPriceInRange(low, high int32) int32 {
	if low <= 0 {
		low = 1
	}
	if high < low {
		high = low
	}
	span := int64(high) - int64(low) + 1
	if span <= 1 {
		return low
	}
	return int32(int64(low) + a.randomInt63n(span))
}

func (a *App) auctionPriceBounds(item catalogItem) (int32, int32) {
	if priceRange, ok := a.customPriceRange(item.ItemID); ok {
		low, high := priceRange.MinPrice, priceRange.MaxPrice
		if valueCategory(item) == valueCategoryEquipment {
			cfg := a.configSnapshot()
			low = clampEquipmentFinalPrice(low, cfg.Restock)
			high = clampEquipmentFinalPrice(high, cfg.Restock)
			if high < low {
				low = high
			}
		}
		return low, high
	}
	cfg := a.configSnapshot()
	center := configuredCenterPrice(item, cfg.Restock)
	lowRand, highRand := cfg.Restock.RandLow, cfg.Restock.RandHigh
	if lowRand <= 0 {
		lowRand = 1
	}
	if highRand < lowRand {
		highRand = lowRand
	}
	low, high := center*lowRand, center*highRand
	category := valueCategory(item)
	if category != valueCategoryEquipment {
		if rule, ok := cfg.Restock.CategoryPriceRules[category]; ok {
			low = math.Max(low, float64(rule.MinPrice))
			high = math.Min(high, float64(rule.MaxPrice))
		}
	}
	if category == valueCategoryEquipment {
		low *= cfg.Restock.EquipmentMultiplierMin
		high *= cfg.Restock.EquipmentMultiplierMax
		if auctionEquipmentCanUpgrade(item) {
			low *= auctionUpgradePriceFactor(cfg.Restock.UpgradeMin, cfg.Restock.UpgradePriceRate)
			high *= auctionUpgradePriceFactor(cfg.Restock.UpgradeMax, cfg.Restock.UpgradePriceRate)
		}
		if max := float64(cfg.Restock.EquipmentFinalMaxPrice); max > 0 && high > max {
			high = max
		}
	}
	return boundedAuctionPrice(low), boundedAuctionPrice(high)
}

func clampEquipmentFinalPrice(price int32, cfg RestockCfg) int32 {
	if price < 1 {
		price = 1
	}
	if cfg.EquipmentFinalMaxPrice > 0 && price > cfg.EquipmentFinalMaxPrice {
		return cfg.EquipmentFinalMaxPrice
	}
	return price
}

func auctionUpgradePriceFactor(upgrade int, rate float64) float64 {
	if upgrade < 0 {
		upgrade = 0
	}
	effectiveLevels := upgrade
	if upgrade > 10 {
		riskLevels := upgrade - 10
		effectiveLevels += riskLevels * riskLevels
	}
	return 1 + float64(effectiveLevels)*rate
}

func boundedAuctionPrice(price float64) int32 {
	if price < 1 {
		return 1
	}
	const maxAuctionPrice = int32(2_000_000_000)
	if price > float64(maxAuctionPrice) {
		return maxAuctionPrice
	}
	return int32(price)
}

func marketBasePrice(item catalogItem) int32 {
	base := item.Price
	if base <= 0 {
		base = item.Value
	}
	if base <= 0 {
		base = 1000
	}
	return base
}

func (a *App) pickOwner(occ map[uint32]int) uint32 {
	cfg := a.configSnapshot()
	owner := cfg.SystemOwner.IDBase
	for occ[owner] >= cfg.SystemOwner.RotateEvery {
		owner++
	}
	occ[owner]++
	return owner
}

func (a *App) nextSpecialAddInfo() int32 {
	cfg := a.configSnapshot()
	a.addInfoMu.Lock()
	defer a.addInfoMu.Unlock()
	if a.specialAddInfo < specialAddInfoBase {
		a.specialAddInfo = specialAddInfoBase
		if a.repository != nil {
			if max, err := a.repository.LoadMaxAddInfo(cfg.AuctionDB, specialAddInfoBase); err == nil && max >= a.specialAddInfo && max < maxInt32 {
				a.specialAddInfo = max + 1
			}
		}
	}
	if a.specialAddInfo <= 0 || a.specialAddInfo >= maxInt32 {
		a.specialAddInfo = specialAddInfoBase
	}
	v := a.specialAddInfo
	a.specialAddInfo++
	return v
}
