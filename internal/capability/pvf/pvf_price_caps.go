package pvf

import (
	"sort"
	"strings"

	"robot/internal/shared"
)

const (
	equipmentPriceCapMaxLevel   = 999
	equipmentPriceCapMinSamples = 12
)

type equipmentPriceSample struct {
	level int
	price int64
}

func buildEquipmentLevelPriceCaps(items []shared.EquipmentCatalogItem) shared.EquipmentLevelPriceCapsDocument {
	samples := make([]equipmentPriceSample, 0, len(items))
	maxLevel := 0
	for _, item := range items {
		price, ok := equipmentPriceCapSample(item)
		if !ok {
			continue
		}
		samples = append(samples, equipmentPriceSample{level: item.Level, price: price})
		if item.Level > maxLevel {
			maxLevel = item.Level
		}
	}
	doc := shared.EquipmentLevelPriceCapsDocument{Version: 1, Method: "rolling-level far-out fences"}
	if len(samples) < 4 {
		return doc
	}
	for level := 0; level <= maxLevel; level++ {
		prices := nearbyEquipmentPrices(samples, level)
		if len(prices) < 4 {
			continue
		}
		sort.Slice(prices, func(i, j int) bool { return prices[i] < prices[j] })
		q1, median, q3 := priceQuartiles(prices)
		iqr := q3 - q1
		doc.Levels = append(doc.Levels, shared.EquipmentLevelPriceCap{
			Level: level, Samples: len(prices), Median: median, Q1: q1, Q3: q3,
			StrictCap: priceCapFence(q3, iqr, 3, 2), StandardCap: priceCapFence(q3, iqr, 3, 1), RelaxedCap: priceCapFence(q3, iqr, 6, 1),
		})
	}
	return doc
}

func equipmentPriceCapSample(item shared.EquipmentCatalogItem) (int64, bool) {
	if item.ID <= 0 || item.Level < 0 || item.Level > equipmentPriceCapMaxLevel || item.BadName || item.Expire || item.ClientIncompatible {
		return 0, false
	}
	slot := strings.ToLower(strings.TrimSpace(item.Slot))
	if item.ItemType == 2 || item.ItemType >= 20 || strings.Contains(slot, "avatar") || slot == "title" || slot == "title name" || slot == "titlename" || slot == "creature" || strings.HasPrefix(slot, "artifact ") {
		return 0, false
	}
	price := item.Price
	if price <= 0 {
		price = item.Value
	}
	if price <= 0 {
		return 0, false
	}
	return int64(price), true
}

func nearbyEquipmentPrices(samples []equipmentPriceSample, target int) []int64 {
	radius := 2
	var prices []int64
	for {
		prices = prices[:0]
		for _, sample := range samples {
			if sample.level >= target-radius && sample.level <= target+radius {
				prices = append(prices, sample.price)
			}
		}
		if len(prices) >= equipmentPriceCapMinSamples || radius >= equipmentPriceCapMaxLevel {
			return prices
		}
		if radius < 5 {
			radius = 5
		} else {
			radius *= 2
			if radius > equipmentPriceCapMaxLevel {
				radius = equipmentPriceCapMaxLevel
			}
		}
	}
}

func priceQuartiles(sorted []int64) (q1, median, q3 int64) {
	median = medianPrice(sorted)
	middle := len(sorted) / 2
	q1 = medianPrice(sorted[:middle])
	upper := sorted[middle:]
	if len(sorted)%2 != 0 {
		upper = sorted[middle+1:]
	}
	q3 = medianPrice(upper)
	return q1, median, q3
}

func medianPrice(sorted []int64) int64 {
	middle := len(sorted) / 2
	if len(sorted)%2 != 0 {
		return sorted[middle]
	}
	return sorted[middle-1] + (sorted[middle]-sorted[middle-1])/2
}

func priceCapFence(q3, iqr int64, numerator, denominator int64) int64 {
	const maxPrice = int64(2_000_000_000)
	cap := q3 + iqr*numerator/denominator
	if cap < 1 {
		return 1
	}
	if cap > maxPrice {
		return maxPrice
	}
	return cap
}
