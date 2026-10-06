package marketapp

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"robot/internal/foundation/marketguard"
)

type BusinessConfig = customPriceRangeDocument

type BusinessItemStatus struct {
	ItemID        uint32 `json:"item_id"`
	Name          string `json:"name"`
	SellPrice     int32  `json:"sell_price"`
	BuyMaxPrice   int32  `json:"buy_max_price"`
	Stock         int64  `json:"stock"`
	Target        int    `json:"target"`
	DailyQuantity int64  `json:"daily_quantity"`
	UsedQuantity  int64  `json:"used_quantity"`
	Reason        string `json:"reason,omitempty"`
}

type BusinessStatus struct {
	Config              BusinessConfig               `json:"config"`
	Items               []BusinessItemStatus         `json:"items"`
	SpentOrReservedGold int64                        `json:"spent_or_reserved_gold"`
	PendingTrades       int                          `json:"pending_trades"`
	Error               string                       `json:"error,omitempty"`
	Legacy              bool                         `json:"legacy"`
	Pending             map[string]marketguard.Trade `json:"pending,omitempty"`
}

func (a *App) BusinessStatus() (BusinessStatus, error) {
	a.refreshCustomPriceRanges()
	b, err := os.ReadFile(a.customPriceRangePath())
	if err != nil {
		return BusinessStatus{}, err
	}
	var status BusinessStatus
	_, priceState := a.businessSnapshot()
	status.Error = priceState.Error
	if err := json.Unmarshal(b, &status.Config); err != nil {
		return status, err
	}
	if status.Config.Version == 1 {
		status.Legacy = true
		status.Config.Version = 2
		status.Config.Limits = defaultBusinessLimits()
		for i := range status.Config.Items {
			r := &status.Config.Items[i]
			r.SellEnabled, r.SellPrice, r.TargetQuantity, r.StackSize, r.UpgradePolicy = r.Enabled, r.MinPrice, 1, 1, "ignore"
			r.Enabled, r.MinPrice, r.MaxPrice = false, 0, 0
		}
	}
	catalog, catalogErr := a.loadCatalog()
	if catalogErr == nil && a.repository != nil {
		if err := a.observeSystemSalePrices(catalog); err != nil {
			status.Error = err.Error()
		}
	}
	ledger, err := marketguard.Snapshot(a.configDir)
	if err != nil {
		return status, err
	}
	day := marketguard.Day(time.Now())
	status.SpentOrReservedGold, _ = marketguard.Usage(ledger, 0, day)
	status.Pending = map[string]marketguard.Trade{}
	for key, t := range ledger.Trades {
		if t.State == "pending" {
			status.PendingTrades++
			status.Pending[key] = t
		}
	}
	stock := map[uint32]int64{}
	if catalogErr != nil {
		status.Error = catalogErr.Error()
	} else if a.repository != nil {
		cfg := a.configSnapshot()
		rows, err := a.repository.LoadSystemCollectRows(cfg.AuctionDB, marketNameAuction, cfg.SystemOwner.IDBase)
		if err != nil {
			status.Error = err.Error()
		} else {
			for _, row := range rows {
				if row.OwnerID >= cfg.SystemOwner.IDBase {
					if n, ok := listingQuantity(row, catalog[row.ItemID]); ok {
						stock[row.ItemID] += int64(n)
					}
				}
			}
		}
	}
	for _, r := range status.Config.Items {
		_, used := marketguard.Usage(ledger, r.ItemID, day)
		item := BusinessItemStatus{ItemID: r.ItemID, Name: r.Name, SellPrice: r.SellPrice, BuyMaxPrice: r.BuyMaxPrice, Stock: stock[r.ItemID], Target: r.TargetQuantity, DailyQuantity: r.BuyDailyQuantityLimit, UsedQuantity: used}
		switch {
		case !r.BuyEnabled:
			item.Reason = "未启用收购"
		case status.Error != "":
			item.Reason = "库存或价格核对失败，停止收购"
		case catalog[r.ItemID].ItemID == 0:
			item.Reason = "当前 PVF 无此物品"
		case ledger.Floors[r.ItemID] > 0 && ledger.Floors[r.ItemID] <= r.BuyMaxPrice:
			item.Reason = "旧库存或摊位低价触发套利保护"
		case used >= r.BuyDailyQuantityLimit:
			item.Reason = "每日数量限额已用完"
		case status.SpentOrReservedGold >= status.Config.Limits.DailyGold:
			item.Reason = "每日金币预算已用完"
		}
		status.Items = append(status.Items, item)
	}
	sort.Slice(status.Items, func(i, j int) bool { return status.Items[i].ItemID < status.Items[j].ItemID })
	return status, nil
}

func (a *App) UpdateBusinessConfig(doc BusinessConfig) (BusinessStatus, error) {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	if doc.Version != 2 {
		return BusinessStatus{}, fmt.Errorf("保存经营清单必须使用 version=2")
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return BusinessStatus{}, err
	}
	ranges, snapshot, err := decodeCustomPriceRanges(b, a.customPriceRangePath(), true)
	if err != nil {
		return BusinessStatus{}, err
	}
	if err := a.validateBusinessEnvironment(ranges); err != nil {
		return BusinessStatus{}, err
	}
	if err := writeJSONFile(a.customPriceRangePath(), doc); err != nil {
		return BusinessStatus{}, err
	}
	a.setPriceRangeState(ranges, snapshot)
	return a.BusinessStatus()
}

func (a *App) validateBusinessEnvironment(rules map[uint32]customPriceRange) error {
	active := false
	for _, r := range rules {
		active = active || r.BuyEnabled || r.SellEnabled
	}
	if !active {
		return nil
	}
	catalog, err := a.loadCatalog()
	if err != nil {
		return err
	}
	info, _, err := a.currentItemInfoIDs()
	if err != nil {
		return err
	}
	if err := a.observeSystemSalePrices(catalog); err != nil {
		return err
	}
	ledger, err := marketguard.Snapshot(a.configDir)
	if err != nil {
		return err
	}
	for id, rule := range rules {
		if !rule.SellEnabled && !rule.BuyEnabled {
			continue
		}
		item, exists := catalog[id]
		if !exists || !info[id] || !marketCandidate(item) {
			return fmt.Errorf("物品 %d 未通过当前 PVF/iteminfo 交易校验", id)
		}
		for upgrade := 0; upgrade <= marketConfigMaxUpgrade; upgrade++ {
			sell, buy := a.businessPrices(rule, item, upgrade)
			if rule.BuyEnabled && rule.SellEnabled && buy > sell {
				return fmt.Errorf("物品 %d 强化 +%d 的实际买卖价存在套利", id, upgrade)
			}
		}
		// 历史底价按物品保守保护，不能用重建或重启抬高底价。
		if rule.BuyEnabled && ledger.Floors[id] > 0 && rule.BuyMaxPrice > ledger.Floors[id] {
			return fmt.Errorf("物品 %d 收购价高于历史系统/摊位售价，需降低收购价或禁收", id)
		}
		if rule.SellEnabled && ledger.Ceilings[id] > rule.SellPrice {
			return fmt.Errorf("物品 %d 新卖价低于历史收购承诺", id)
		}
	}
	return nil
}
