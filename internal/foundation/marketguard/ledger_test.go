package marketguard

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"robot/internal/foundation/layout"
)

func testRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := layout.New(root).Ensure(); err != nil {
		t.Fatal(err)
	}
	return root
}
func testLimits() Limits {
	return Limits{DailyGold: 1000, DailyQuantity: 10, MaxOrderGold: 200, Ceiling: 100, SaleFloor: 200}
}

func TestConcurrentReservationsAndRestart(t *testing.T) {
	root := testRoot(t)
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if Reserve(root, fmt.Sprint(i), Trade{ItemID: 1, Quantity: 1, Gold: 100, Day: "2026-10-05"}, testLimits()) == nil {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("并发预留成功 %d 次，期望 10", accepted.Load())
	}
	l, err := Snapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	gold, count := Usage(l, 1, "2026-10-05")
	if gold != 1000 || count != 10 {
		t.Fatalf("预算数量=%d/%d", gold, count)
	}
	if err := Reserve(root, "new", Trade{ItemID: 1, Quantity: 1, Gold: 100, Day: "2026-10-06"}, testLimits()); err == nil {
		t.Fatal("跨日绕过未确认预留")
	}
}

func TestExactLimitFailureReleaseAndIdempotentConfirmation(t *testing.T) {
	root := testRoot(t)
	trade := Trade{ItemID: 1, Quantity: 2, Gold: 200, Day: "2026-10-05"}
	if err := Reserve(root, "a", trade, testLimits()); err != nil {
		t.Fatal(err)
	}
	if err := Reserve(root, "a", trade, testLimits()); err == nil {
		t.Fatal("重复预留")
	}
	if err := Finish(root, "a", false); err != nil {
		t.Fatal(err)
	}
	if err := Reserve(root, "a", trade, testLimits()); err != nil {
		t.Fatal(err)
	}
	if err := Finish(root, "a", true); err != nil {
		t.Fatal(err)
	}
	if err := Finish(root, "a", true); err != nil {
		t.Fatal(err)
	}
	l, _ := Snapshot(root)
	g, q := Usage(l, 1, "2026-10-05")
	if g != 200 || q != 2 {
		t.Fatalf("重复记账 %d/%d", g, q)
	}
	if g, q := Usage(l, 1, "2026-10-06"); g != 0 || q != 0 {
		t.Fatal("已确认交易未按日重置")
	}
	trade.Gold = 201
	if err := Reserve(root, "b", trade, testLimits()); err == nil {
		t.Fatal("单价尾数超限被接受")
	}
}

func TestOldStockAndStorePricesProtectBothDirections(t *testing.T) {
	root := testRoot(t)
	if err := ObserveSales(root, map[uint32]int32{1: 90}); err != nil {
		t.Fatal(err)
	}
	if err := RecordSales(root, map[uint32]int32{1: 300}); err != nil {
		t.Fatal(err)
	}
	trade := Trade{ItemID: 1, Quantity: 1, Gold: 80, Day: "2026-10-05"}
	if err := Reserve(root, "a", trade, testLimits()); err == nil {
		t.Fatal("提高卖价遗忘旧低价库存")
	}
	trade.ItemID = 2
	if err := Reserve(root, "b", trade, testLimits()); err != nil {
		t.Fatal(err)
	}
	if err := RecordSales(root, map[uint32]int32{2: 99}); err == nil {
		t.Fatal("摊位价低于已预留收购上限")
	}
	if err := RecordSales(root, map[uint32]int32{2: 100}); err != nil {
		t.Fatal("摊位价等于收购上限应允许")
	}
	if err := RecordSales(root, map[uint32]int32{2: 101}); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptLedgerFailsClosedAndBeijingDay(t *testing.T) {
	root := testRoot(t)
	p, _ := path(root)
	if err := os.WriteFile(p, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Reserve(root, "a", Trade{ItemID: 1, Quantity: 1, Gold: 10, Day: "2026-10-05"}, testLimits()); err == nil {
		t.Fatal("损坏账本未阻止购买")
	}
	if got := Day(time.Date(2026, 10, 5, 16, 0, 0, 0, time.UTC)); got != "2026-10-06" {
		t.Fatal(got)
	}
}
