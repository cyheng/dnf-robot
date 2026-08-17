package marketapp

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

func (a *App) auctionUnitPriceFor(item catalogItem, batchInflate float64, upgrade int) int32 {
	cfg := a.configSnapshot()
	if item.ItemID > 0 {
		if priceRange, ok := a.customPriceRange(item.ItemID); ok {
			return a.randomPriceInRange(priceRange.MinPrice, priceRange.MaxPrice)
		}
	}
	price := valueModelCenterPriceWithConfig(item, cfg.Restock)
	if item.Kind == "equipment" {
		if batchInflate <= 0 {
			batchInflate = 1
		}
		price *= batchInflate
		price *= auctionUpgradePriceFactor(upgrade, cfg.Restock.UpgradePriceRate)
	}
	low, high := cfg.Restock.RandLow, cfg.Restock.RandHigh
	if low > 0 && high > 0 && low != high {
		if high < low {
			high = low
		}
		price *= low + a.randomFloat64()*(high-low)
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
		return priceRange.MinPrice, priceRange.MaxPrice
	}
	cfg := a.configSnapshot()
	center := valueModelCenterPriceWithConfig(item, cfg.Restock)
	lowRand, highRand := cfg.Restock.RandLow, cfg.Restock.RandHigh
	if lowRand <= 0 {
		lowRand = 1
	}
	if highRand < lowRand {
		highRand = lowRand
	}
	low, high := center*lowRand, center*highRand
	if item.Kind == "equipment" {
		low *= float64(cfg.Restock.EquipInflateMin)
		high *= float64(cfg.Restock.EquipInflateMax)
		if auctionEquipmentCanUpgrade(item) {
			low *= auctionUpgradePriceFactor(cfg.Restock.UpgradeMin, cfg.Restock.UpgradePriceRate)
			high *= auctionUpgradePriceFactor(cfg.Restock.UpgradeMax, cfg.Restock.UpgradePriceRate)
		}
	}
	return boundedAuctionPrice(low), boundedAuctionPrice(high)
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
