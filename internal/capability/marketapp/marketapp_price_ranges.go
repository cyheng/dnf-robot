package marketapp

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"time"

	foundationconfig "robot/internal/foundation/config"
	"robot/internal/foundation/layout"
)

//go:embed default_business_list.json
var embeddedDefaultBusinessList []byte

type customPriceRangeDocument struct {
	Description string             `json:"description"`
	FieldNotes  map[string]string  `json:"field_notes"`
	Version     int                `json:"version"`
	Limits      BusinessLimits     `json:"limits"`
	Items       []customPriceRange `json:"items"`
}

func defaultCustomPriceRangeDocument() customPriceRangeDocument {
	if len(embeddedDefaultBusinessList) > 0 {
		var doc customPriceRangeDocument
		if err := json.Unmarshal(embeddedDefaultBusinessList, &doc); err == nil && doc.Version == 2 && len(doc.Items) > 0 {
			return doc
		}
	}
	return customPriceRangeDocument{
		Description: "拍卖行物品经营清单。单品买卖分价，未配置默认禁收禁售。v1 仅兼容原卖价，不产生收购权限。旧挂单保留，低价历史库存触发禁收。v2 卖价固定，不使用通用随机折扣或装备倍率。",
		FieldNotes: map[string]string{
			"item_id":        "用于匹配的 DNF 物品 ID。",
			"name":           "可选备注名称，仅供管理员识别，不参与匹配。",
			"min_price":      "最终最低单价；堆叠物品按单件计算，装备按单条拍卖记录计算。",
			"max_price":      "最终最高单价，必须大于或等于 min_price。",
			"enabled":        "是否启用该物品的独立价格覆盖。",
			"sell_enabled":   "是否补货；target_quantity 为目标总数量，stack_size 为每条数量，装备始终一件。",
			"sell_price":     "基础系统出售单价；升级策略为 actual 时按实际强化和全局强化率同时调整买卖价。",
			"buy_max_price":  "最高收购单价，不是付款额；必须严格低于所有系统最低售价。",
			"buy_enabled":    "是否收购；buy_daily_quantity_limit 必须为正数。",
			"upgrade_policy": "ignore 忽略强化；actual 按挂单实际强化加价，读取不到按 +0，称号等不加价。",
			"limits":         "max_actions 每轮上限、max_order_gold 单笔上限、daily_gold 每日金币预算；均必须为正数。",
		},
		Version: 2,
		Limits:  defaultBusinessLimits(),
		Items:   []customPriceRange{},
	}
}

func (a *App) customPriceRangePath() string {
	return layout.New(a.configDir).MarketPrices()
}

func (a *App) refreshCustomPriceRanges() {
	if a.runtimeFilesWatched.Load() {
		return
	}
	path := a.customPriceRangePath()
	enabled := a.configSnapshot().Restock.CustomPriceEnabled

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if writeErr := writeJSONFile(path, defaultCustomPriceRangeDocument()); writeErr != nil {
			a.setPriceRangeState(nil, PriceRangeStatus{Enabled: enabled, Path: path, Error: writeErr.Error()})
			return
		}
	}
	ranges, status, err := readCustomPriceRanges(path, enabled)
	if err != nil {
		a.setPriceRangeState(nil, PriceRangeStatus{Enabled: enabled, Path: path, Error: err.Error()})
		return
	}
	a.setPriceRangeState(ranges, status)
}

func (a *App) reloadCustomPriceRangeFile(path string) error {
	expected := a.customPriceRangePath()
	if path != expected {
		return nil
	}
	enabled := a.configSnapshot().Restock.CustomPriceEnabled
	// 文件被删除时重建默认清单，而不是报错挂起经营；与启动时 refreshCustomPriceRanges 的行为一致。
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if writeErr := writeJSONFile(path, defaultCustomPriceRangeDocument()); writeErr != nil {
			a.stateMu.Lock()
			a.priceRangeStatus.Error = writeErr.Error()
			a.stateMu.Unlock()
			return writeErr
		}
	}
	ranges, status, err := readCustomPriceRanges(path, enabled)
	if err != nil {
		a.stateMu.Lock()
		a.priceRangeStatus.Error = err.Error()
		a.stateMu.Unlock()
		return err
	}
	a.setPriceRangeState(ranges, status)
	a.appendLog(LogEvent{Type: "config", Status: marketLogStatusSuccess, Message: fmt.Sprintf("market price ranges reloaded: items=%d", status.LoadedItems)})
	return nil
}

func readCustomPriceRanges(path string, enabled bool) (map[uint32]customPriceRange, PriceRangeStatus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, PriceRangeStatus{}, err
	}
	return decodeCustomPriceRanges(data, path, enabled)
}

func decodeCustomPriceRanges(data []byte, path string, enabled bool) (map[uint32]customPriceRange, PriceRangeStatus, error) {
	var doc struct {
		Description string              `json:"description"`
		FieldNotes  map[string]string   `json:"field_notes"`
		Version     int                 `json:"version"`
		Limits      BusinessLimits      `json:"limits"`
		Items       *[]customPriceRange `json:"items"`
	}
	if err := foundationconfig.DecodeJSONBytes(data, &doc); err != nil {
		return nil, PriceRangeStatus{}, err
	}
	if (doc.Version != 1 && doc.Version != 2) || doc.Items == nil {
		return nil, PriceRangeStatus{}, fmt.Errorf("unsupported or incomplete price range document")
	}
	if doc.Version == 2 {
		if err := validateBusinessLimits(doc.Limits); err != nil {
			return nil, PriceRangeStatus{}, err
		}
	}
	ranges := make(map[uint32]customPriceRange, len(*doc.Items))
	seen := make(map[uint32]struct{}, len(*doc.Items))
	for index, row := range *doc.Items {
		if row.ItemID == 0 || doc.Version == 1 && (row.MinPrice <= 0 || row.MaxPrice < row.MinPrice) {
			return nil, PriceRangeStatus{}, fmt.Errorf("invalid item price range at index %d", index)
		}
		if doc.Version == 2 {
			if err := validateBusinessItem(row); err != nil {
				return nil, PriceRangeStatus{}, err
			}
		}
		if _, exists := seen[row.ItemID]; exists {
			return nil, PriceRangeStatus{}, fmt.Errorf("duplicate item price range for item_id %d", row.ItemID)
		}
		seen[row.ItemID] = struct{}{}
		if row.Enabled || doc.Version == 2 {
			ranges[row.ItemID] = row
		}
	}
	if !enabled && doc.Version == 1 {
		return map[uint32]customPriceRange{}, PriceRangeStatus{Version: 1, Enabled: false, Path: path, LoadedAt: time.Now()}, nil
	}
	status := PriceRangeStatus{Version: doc.Version, Limits: doc.Limits, Enabled: true, Path: path, LoadedItems: len(ranges), LoadedAt: time.Now()}
	return ranges, status, nil
}

func (a *App) setPriceRangeState(ranges map[uint32]customPriceRange, status PriceRangeStatus) {
	if ranges == nil {
		ranges = map[uint32]customPriceRange{}
	}
	a.stateMu.Lock()
	a.priceRanges = ranges
	a.priceRangeStatus = status
	a.stateMu.Unlock()
}

func (a *App) customPriceRange(itemID uint32) (customPriceRange, bool) {
	if !a.configSnapshot().Restock.CustomPriceEnabled {
		return customPriceRange{}, false
	}
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	rangeCfg, ok := a.priceRanges[itemID]
	return rangeCfg, ok && a.priceRangeStatus.Version == 1
}
