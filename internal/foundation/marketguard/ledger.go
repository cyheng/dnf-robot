// Package marketguard 统一记录拍卖与摊位的价格底线和收购预留。
package marketguard

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"robot/internal/foundation/atomicfile"
	"robot/internal/foundation/layout"
	"robot/internal/foundation/lockhub"
)

// 同一进程中的摊位、手动及自动拍卖任务共享此锁；部署只运行一个 Robot。
var mu lockhub.Locker

type Trade struct {
	ItemID   uint32 `json:"item_id"`
	Quantity int64  `json:"quantity"`
	Gold     int64  `json:"gold"`
	Day      string `json:"day"`
	State    string `json:"state"`
}

type Ledger struct {
	Version  int              `json:"version"`
	Floors   map[uint32]int32 `json:"minimum_sale_prices"`
	Ceilings map[uint32]int32 `json:"purchase_ceiling_history"`
	Trades   map[string]Trade `json:"trades"`
}

type Limits struct {
	DailyGold     int64
	DailyQuantity int64
	MaxOrderGold  int64
	Ceiling       int32
	SaleFloor     int32
}

func Day(now time.Time) string {
	return now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
}

func path(root string) (string, error) {
	p := layout.New(root)
	if !p.Valid() {
		return "", fmt.Errorf("市场账本需要绝对配置路径")
	}
	return p.MarketPurchaseLedger(), nil
}

func read(root string) (Ledger, error) {
	l := Ledger{Version: 1, Floors: map[uint32]int32{}, Ceilings: map[uint32]int32{}, Trades: map[string]Trade{}}
	p, err := path(root)
	if err != nil {
		return l, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return l, err
	}
	l = Ledger{}
	if err = json.Unmarshal(b, &l); err != nil {
		return l, err
	}
	if l.Version != 1 || l.Floors == nil || l.Ceilings == nil || l.Trades == nil {
		return l, fmt.Errorf("市场账本不完整，停止交易")
	}
	for id, price := range l.Floors {
		if id == 0 || price <= 0 {
			return l, fmt.Errorf("市场账本历史售价无效")
		}
	}
	for id, price := range l.Ceilings {
		if id == 0 || price <= 0 {
			return l, fmt.Errorf("市场账本历史收购价无效")
		}
	}
	for _, t := range l.Trades {
		if t.ItemID == 0 || t.Quantity <= 0 || t.Gold <= 0 || t.Day == "" || (t.State != "pending" && t.State != "confirmed") {
			return l, fmt.Errorf("市场账本交易无效")
		}
	}
	return l, nil
}

func write(root string, l Ledger) error {
	p, err := path(root)
	if err != nil {
		return err
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(p, b, 0600)
}

func Snapshot(root string) (Ledger, error) {
	mu.Lock()
	defer mu.Unlock()
	return read(root)
}

// ObserveSales 记录历史库存；低价旧库存不能因为改配置或重启而被遗忘。
func ObserveSales(root string, prices map[uint32]int32) error {
	return recordSales(root, prices, false)
}

// RecordSales 必须在上架前调用；低于已经承诺的收购上限时禁止新上架。
func RecordSales(root string, prices map[uint32]int32) error { return recordSales(root, prices, true) }

func recordSales(root string, prices map[uint32]int32, enforce bool) error {
	mu.Lock()
	defer mu.Unlock()
	l, err := read(root)
	if err != nil {
		return err
	}
	changed := false
	for id, price := range prices {
		if id == 0 || price <= 0 {
			return fmt.Errorf("无效系统售价 item=%d", id)
		}
		if enforce && l.Ceilings[id] > price {
			return fmt.Errorf("套利保护：物品 %d 售价 %d 低于已承诺收购价 %d", id, price, l.Ceilings[id])
		}
		if l.Floors[id] == 0 || price < l.Floors[id] {
			l.Floors[id] = price
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return write(root, l)
}

func Usage(l Ledger, itemID uint32, day string) (gold, quantity int64) {
	for _, t := range l.Trades {
		// 不确定结果跨日仍占预算，不能靠等待午夜绕过保护。
		if t.Day != day && t.State != "pending" {
			continue
		}
		gold += t.Gold
		if t.ItemID == itemID {
			quantity += t.Quantity
		}
	}
	return
}

func Check(l Ledger, key string, t Trade, limits Limits) error {
	if _, exists := l.Trades[key]; exists {
		return fmt.Errorf("交易已执行或等待核对：%s", key)
	}
	if t.ItemID == 0 || t.Quantity <= 0 || t.Gold <= 0 || limits.Ceiling <= 0 || limits.DailyGold <= 0 || limits.DailyQuantity <= 0 || limits.MaxOrderGold <= 0 {
		return fmt.Errorf("缺少有效的收购硬限制")
	}
	if t.Gold > limits.MaxOrderGold || t.Gold > int64(limits.Ceiling)*t.Quantity {
		return fmt.Errorf("超过单价或单笔收购上限")
	}
	floor := limits.SaleFloor
	if old := l.Floors[t.ItemID]; old > 0 && (floor <= 0 || old < floor) {
		floor = old
	}
	if floor > 0 && limits.Ceiling > floor {
		return fmt.Errorf("套利保护：最高收购价 %d 不能高于最低历史系统售价 %d", limits.Ceiling, floor)
	}
	gold, quantity := Usage(l, t.ItemID, t.Day)
	if t.Gold > limits.DailyGold-gold {
		return fmt.Errorf("每日金币预算不足")
	}
	if t.Quantity > limits.DailyQuantity-quantity {
		return fmt.Errorf("物品每日收购数量已达上限")
	}
	return nil
}

func Reserve(root, key string, t Trade, limits Limits) error {
	mu.Lock()
	defer mu.Unlock()
	l, err := read(root)
	if err != nil {
		return err
	}
	if err = Check(l, key, t, limits); err != nil {
		return err
	}
	t.State = "pending"
	l.Trades[key] = t
	if limits.Ceiling > l.Ceilings[t.ItemID] {
		l.Ceilings[t.ItemID] = limits.Ceiling
	}
	return write(root, l)
}

// Finish 仅在收到明确协议结果时提交或释放；断线时保持 pending。
func Finish(root, key string, success bool) error {
	mu.Lock()
	defer mu.Unlock()
	l, err := read(root)
	if err != nil {
		return err
	}
	t, ok := l.Trades[key]
	if !ok || t.State == "confirmed" {
		return nil
	}
	if success {
		t.State = "confirmed"
		l.Trades[key] = t
	} else {
		delete(l.Trades, key)
	}
	return write(root, l)
}
