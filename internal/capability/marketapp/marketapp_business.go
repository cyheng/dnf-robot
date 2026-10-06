package marketapp

import (
	"fmt"
	"sort"
	"time"

	"robot/internal/foundation/marketguard"
)

type BusinessLimits struct {
	MaxActions   int   `json:"max_actions"`
	MaxOrderGold int64 `json:"max_order_gold"`
	DailyGold    int64 `json:"daily_gold"`
}

func defaultBusinessLimits() BusinessLimits {
	return BusinessLimits{MaxActions: 100, MaxOrderGold: 50_000_000, DailyGold: 100_000_000}
}

func validateBusinessLimits(l BusinessLimits) error {
	if l.MaxActions <= 0 || l.MaxActions > 10000 || l.MaxOrderGold <= 0 || l.MaxOrderGold > 2_000_000_000 || l.DailyGold <= 0 || l.DailyGold > 1_000_000_000_000 {
		return fmt.Errorf("limits 需要有效的动作上限、单笔总额及每日预算")
	}
	return nil
}

func validateBusinessItem(r customPriceRange) error {
	if r.ItemID == 0 || (r.UpgradePolicy != "ignore" && r.UpgradePolicy != "actual") {
		return fmt.Errorf("item %d 的 upgrade_policy 必须为 ignore 或 actual", r.ItemID)
	}
	if r.SellEnabled && (r.SellPrice <= 0 || r.SellPrice > 2_000_000_000 || r.TargetQuantity <= 0 || r.TargetQuantity > 1_000_000 || r.StackSize <= 0 || r.StackSize > 1_000_000) {
		return fmt.Errorf("item %d 的卖价、库存或堆叠数量无效", r.ItemID)
	}
	if r.BuyEnabled && (r.BuyMaxPrice <= 0 || r.BuyMaxPrice > 2_000_000_000 || r.BuyDailyQuantityLimit <= 0 || r.BuyDailyQuantityLimit > 1_000_000) {
		return fmt.Errorf("item %d 的收购上限或每日数量无效", r.ItemID)
	}
	if r.SellEnabled && r.BuyEnabled && r.BuyMaxPrice > r.SellPrice {
		return fmt.Errorf("item %d 最高收购价不能高于系统卖价", r.ItemID)
	}
	return nil
}

func (a *App) businessSnapshot() (map[uint32]customPriceRange, PriceRangeStatus) {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	// 文件快照发布后只读，不在业务路径修改。
	return a.priceRanges, a.priceRangeStatus
}

func (a *App) businessPrices(r customPriceRange, item catalogItem, upgrade int) (sell, buy int32) {
	cfg := a.configSnapshot()
	factor := 1.0
	if r.UpgradePolicy == "actual" && auctionEquipmentCanUpgrade(item) {
		if upgrade < 0 || upgrade > marketConfigMaxUpgrade {
			upgrade = 0
		}
		factor = auctionUpgradePriceFactor(upgrade, cfg.Restock.UpgradePriceRate)
	}
	if r.SellEnabled {
		sell = boundedAuctionPrice(float64(r.SellPrice) * factor)
	}
	if r.BuyEnabled {
		buy = boundedAuctionPrice(float64(r.BuyMaxPrice) * factor)
	}
	if valueCategory(item) == valueCategoryEquipment && sell > 0 {
		sell = clampEquipmentFinalPrice(sell, cfg.Restock)
	}
	return
}

// v2 清单独立于旧价格过滤开关；缺失、损坏或版本不明时不会扩大经营范围。
func (a *App) planBusinessAuction(req RestockRequest, catalog map[uint32]catalogItem, pvfReady bool, occ map[uint32]int, result *PlanResult) error {
	rules, status := a.businessSnapshot()
	if status.Error != "" {
		return fmt.Errorf("经营配置无效：%s", status.Error)
	}
	if !pvfReady {
		return fmt.Errorf("经营清单补货需要当前 PVF")
	}
	info, _, err := a.currentItemInfoIDs()
	if err != nil {
		return err
	}
	cfg := a.configSnapshot()
	stock, err := a.repository.LoadSystemCollectRows(cfg.AuctionDB, marketNameAuction, cfg.SystemOwner.IDBase)
	if err != nil {
		return err
	}
	have := map[uint32]int{}
	for _, row := range stock {
		if row.OwnerID < cfg.SystemOwner.IDBase {
			continue
		}
		item, ok := catalog[row.ItemID]
		if !ok {
			continue
		}
		count, valid := listingQuantity(row, item)
		if !valid {
			return fmt.Errorf("系统库存数量无效 item=%d", row.ItemID)
		}
		have[row.ItemID] += int(count)
	}
	ids := make([]uint32, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	requested := idSet(req.ItemIDs)
	maxActions := req.MaxActions
	if maxActions <= 0 {
		maxActions = cfg.Restock.MaxActions
	}
	if maxActions <= 0 {
		maxActions = defaultMarketMaxActions
	}
	for _, id := range ids {
		rule := rules[id]
		if !rule.SellEnabled || len(requested) > 0 && !requested[id] {
			continue
		}
		item, known := catalog[id]
		reason := ""
		if !known {
			reason = "missing_from_pvf"
		} else if !info[id] {
			reason = "missing_iteminfo"
		} else if !a.marketCandidate(item) {
			reason = "not_auctionable"
		}
		if reason != "" {
			result.Skipped = append(result.Skipped, SkippedItem{Market: marketNameAuction, ItemID: id, Name: rule.Name, Reason: reason})
			continue
		}
		gap := rule.TargetQuantity - have[id]
		if gap <= 0 {
			continue
		}
		row := restockRow{ItemID: id, Name: rule.Name, SystemPrice: rule.SellPrice, Quantity: gap, StackSize: rule.StackSize, Enabled: true, Source: "business_list"}
		row.applyMarketItem(item)
		remaining := maxActions - len(result.Actions)
		if remaining <= 0 {
			break
		}
		stack := auctionPlanStackSize(row, item, item.Kind == "equipment")
		// 防止价格溢出缩堆后漏补，同时限制生成动作的内存占用。
		maxPrice, _ := a.businessPrices(rule, item, cfg.Restock.UpgradeMax)
		stack = int(safeAuctionStackCount(maxPrice, int32(stack)))
		row.StackSize = stack
		if row.Quantity > remaining*stack {
			row.Quantity = remaining * stack
		}
		a.planAuction([]restockRow{row}, catalog, map[uint32]int{}, occ, result)
	}
	return nil
}

func listingQuantity(row collectRow, item catalogItem) (int32, bool) {
	if item.Kind == "equipment" || specialAuctionKind(item) != "" {
		return 1, true
	}
	if item.Kind != "stackable" || row.Count <= 0 {
		return 0, false
	}
	return row.Count, true
}

func (a *App) purchaseTerms(row collectRow, item catalogItem) (Action, marketguard.Trade, marketguard.Limits, error) {
	cfg := a.configSnapshot()
	rules, status := a.businessSnapshot()
	rule, exists := rules[row.ItemID]
	if status.Error != "" || status.Version != 2 || !exists || !rule.BuyEnabled {
		return Action{}, marketguard.Trade{}, marketguard.Limits{}, fmt.Errorf("物品未配置收购或经营配置无效")
	}
	if row.OwnerID == 0 || row.OwnerID >= cfg.SystemOwner.IDBase {
		return Action{}, marketguard.Trade{}, marketguard.Limits{}, fmt.Errorf("排除系统或无效卖家")
	}
	if row.AuctionID == 0 || row.InstantPrice <= 0 {
		return Action{}, marketguard.Trade{}, marketguard.Limits{}, fmt.Errorf("只收购有效的一口价挂单")
	}
	count, valid := listingQuantity(row, item)
	if !valid {
		return Action{}, marketguard.Trade{}, marketguard.Limits{}, fmt.Errorf("无法确认物品类型或数量")
	}
	// 堆叠物 price=-1 是分堆挂单，需要另一种购买协议，本执行器只支持整单 Bid。
	// 装备 price=-1 是纯一口价（无竞拍底价），整单 Bid 可买，不视为分堆。
	if row.StartPrice < 0 && item.Kind != "equipment" {
		return Action{}, marketguard.Trade{}, marketguard.Limits{}, fmt.Errorf("当前收购协议不支持分堆挂单")
	}
	upgrade := 0
	if row.Upgrade != nil {
		upgrade = *row.Upgrade
	}
	sell, buy := a.businessPrices(rule, item, upgrade)
	limits := marketguard.Limits{DailyGold: status.Limits.DailyGold, DailyQuantity: rule.BuyDailyQuantityLimit, MaxOrderGold: status.Limits.MaxOrderGold, Ceiling: buy, SaleFloor: sell}
	trade := marketguard.Trade{ItemID: row.ItemID, Quantity: int64(count), Gold: int64(row.InstantPrice), Day: marketguard.Day(time.Now())}
	// 使用总额比较而非向下取整的单价，避免材料尾数绕过硬上限。
	if row.InstantPrice%count != 0 {
		return Action{}, trade, limits, fmt.Errorf("材料总价与数量不能形成整数单价")
	}
	action := Action{Market: marketNameAuction, Kind: "collect", Operation: "collect", ItemID: row.ItemID, Name: item.Name, Count: count, CountAddInfo: row.Count, UnitPrice: row.InstantPrice / count, TotalPrice: row.InstantPrice, InstantPrice: row.InstantPrice, StartPrice: row.StartPrice, SellerID: row.OwnerID, OwnerID: cfg.SystemOwner.BuyerBase, OwnerName: cfg.SystemOwner.OwnerName, Upgrade: &upgrade, AuctionID: row.AuctionID, Source: "auction_main"}
	return action, trade, limits, nil
}

func (a *App) tradeKey(id uint64) string {
	return fmt.Sprintf("%s:%d", a.configSnapshot().AuctionDB, id)
}
