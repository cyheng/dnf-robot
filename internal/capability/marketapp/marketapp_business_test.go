package marketapp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"robot/internal/foundation/marketguard"
)

func businessTestApp(t *testing.T) (*App, *clearStockRepository) {
	t.Helper()
	a := testApp(t)
	a.cfg.Restock.UpgradeMin = 0
	a.cfg.Restock.UpgradeMax = 0
	r := &clearStockRepository{collectRows: map[string][]collectRow{}, systemCollectRows: map[string][]collectRow{}}
	a.repository = r
	mustWriteJSON(t, appPaths(a).PVFEquipment(), []map[string]interface{}{{"id": 1001, "name": "普通紫装", "slot": "weapon", "attach": "trade", "rarity": 2}, {"id": 1002, "name": "墨竹", "slot": "wrist", "attach": "trade", "rarity": 2}, {"id": 1003, "name": "骨戒", "slot": "ring", "attach": "trade", "rarity": 3}})
	mustWriteJSON(t, appPaths(a).PVFStackable(), []map[string]interface{}{{"id": 4000, "name": "材料", "attach": "free", "stack_limit": 1000}})
	a.cfg.ItemInfoTargets = []string{appPaths(a).PVFItemInfo()}
	mustWriteText(t, appPaths(a).PVFItemInfo(), "1001 1 `普通紫装`\n1002 1 `墨竹`\n1003 1 `骨戒`\n4000 0 `材料`\n")
	doc := defaultCustomPriceRangeDocument()
	doc.Items = []customPriceRange{
		{ItemID: 1001, Name: "普通紫装", SellEnabled: true, SellPrice: 500000, TargetQuantity: 10, StackSize: 1, BuyEnabled: true, BuyMaxPrice: 100000, BuyDailyQuantityLimit: 100, UpgradePolicy: "ignore"},
		{ItemID: 1002, Name: "墨竹", SellEnabled: true, SellPrice: 3000000, TargetQuantity: 1, StackSize: 1, BuyEnabled: true, BuyMaxPrice: 1500000, BuyDailyQuantityLimit: 100, UpgradePolicy: "ignore"},
		{ItemID: 1003, Name: "骨戒", SellEnabled: true, SellPrice: 80000000, TargetQuantity: 1, StackSize: 1, BuyEnabled: true, BuyMaxPrice: 40000000, BuyDailyQuantityLimit: 100, UpgradePolicy: "ignore"},
		{ItemID: 4000, Name: "材料", SellEnabled: true, SellPrice: 200, TargetQuantity: 1000, StackSize: 100, BuyEnabled: true, BuyMaxPrice: 100, BuyDailyQuantityLimit: 1000, UpgradePolicy: "ignore"},
	}
	mustWriteJSON(t, appPaths(a).MarketPrices(), doc)
	a.refreshCustomPriceRanges()
	return a, r
}

func playerRow(id uint64, item uint32, price int32) collectRow {
	return collectRow{Market: marketNameAuction, AuctionID: id, OwnerID: 12, ItemID: item, Count: 23207, StartPrice: 1, InstantPrice: price}
}

func TestBusinessPurchaseHardCapsAcrossOneHundredScans(t *testing.T) {
	a, r := businessTestApp(t)
	a.cfg.Collector.OutRangeProbability = 1
	r.collectRows[a.cfg.AuctionDB] = []collectRow{playerRow(1, 1001, 80000), playerRow(2, 1001, 100000), playerRow(3, 1001, 110000), playerRow(4, 1001, 100000000), playerRow(5, 1002, 1500000), playerRow(6, 1003, 20000000), playerRow(7, 1003, 60000000), playerRow(8, 9999, 1)}
	for i := 0; i < 100; i++ {
		a.cfg.Collector.PriceRangeEnabled = i%2 == 0
		a.cfg.Collector.InRangeProbability = 1
		plan, err := a.CollectPlan(CollectRequest{Market: "auction"})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Actions) != 4 {
			t.Fatalf("第%d轮 actions=%+v skipped=%+v", i, plan.Actions, plan.Skipped)
		}
		for _, act := range plan.Actions {
			if act.AuctionID == 3 || act.AuctionID == 4 || act.AuctionID == 7 || act.AuctionID == 8 {
				t.Fatalf("越限购买 %+v", act)
			}
			if act.Count != 1 {
				t.Fatal("装备 add_info 被当成数量")
			}
			if act.ItemID == 1003 && act.TotalPrice != 20000000 {
				t.Fatal("按最高价付款")
			}
		}
	}
}

func TestBusinessMaterialQuantityAndUnsupportedBuyouts(t *testing.T) {
	a, r := businessTestApp(t)
	row := playerRow(1, 4000, 1000)
	row.Count = 10
	r.collectRows[a.cfg.AuctionDB] = []collectRow{row}
	plan, err := a.CollectPlan(CollectRequest{Market: "auction"})
	if err != nil || len(plan.Actions) != 1 {
		t.Fatalf("%v %+v", err, plan)
	}
	if act := plan.Actions[0]; act.Count != 10 || act.UnitPrice != 100 || act.TotalPrice != 1000 {
		t.Fatalf("%+v", act)
	}
	for _, test := range []struct{ count, price, start int32 }{{10, 1001, 1}, {0, 10, 1}, {10, -1, 100}, {10, 1000, -1}} {
		row.Count, row.InstantPrice, row.StartPrice = test.count, test.price, test.start
		r.collectRows[a.cfg.AuctionDB] = []collectRow{row}
		plan, _ := a.CollectPlan(CollectRequest{Market: "auction"})
		if len(plan.Actions) > 0 {
			t.Fatalf("接受异常或不支持订单 %+v", test)
		}
	}
}

func TestBusinessEquipmentBuyoutWithNegativeStartPrice(t *testing.T) {
	a, r := businessTestApp(t)
	// 装备纯一口价（price=-1，无竞拍底价）应能收购，不视为分堆。
	row := playerRow(1, 1001, 80000)
	row.StartPrice = -1
	r.collectRows[a.cfg.AuctionDB] = []collectRow{row}
	plan, err := a.CollectPlan(CollectRequest{Market: "auction"})
	if err != nil || len(plan.Actions) != 1 {
		t.Fatalf("装备纯一口价应能收购：%v %+v", err, plan)
	}
}

func TestBusinessUsesActualUpgradeAndExcludesSystemOwners(t *testing.T) {
	a, r := businessTestApp(t)
	rules, status := a.businessSnapshot()
	rule := rules[1001]
	rule.UpgradePolicy = "actual"
	rules[1001] = rule
	a.setPriceRangeState(rules, status)
	a.cfg.Restock.UpgradeMin = 13
	a.cfg.Restock.UpgradeMax = 13
	catalog, _ := a.loadCatalog()
	row := playerRow(1, 1001, 200000)
	_, trade, limits, err := a.purchaseTerms(row, catalog[1001])
	if err != nil {
		t.Fatal(err)
	}
	if limits.Ceiling != 100000 {
		t.Fatalf("白板用了补货强化 %d", limits.Ceiling)
	}
	l, _ := marketguard.Snapshot(a.configDir)
	if marketguard.Check(l, "a", trade, limits) == nil {
		t.Fatal("白板超价收购")
	}
	upgrade := 13
	row.Upgrade = &upgrade
	_, _, limits, err = a.purchaseTerms(row, catalog[1001])
	if err != nil || limits.Ceiling != 276000 {
		t.Fatalf("实际强化没有加价 %d %v", limits.Ceiling, err)
	}
	row.OwnerID = a.cfg.SystemOwner.IDBase
	r.collectRows[a.cfg.AuctionDB] = []collectRow{row}
	a.cfg.Collector.IncludeSystemOwners = true
	plan, _ := a.CollectPlan(CollectRequest{Market: "auction"})
	if len(plan.Actions) > 0 {
		t.Fatal("系统卖家进入玩家收购")
	}
}

func TestBusinessRestockFillsOnlyRealSystemGap(t *testing.T) {
	a, r := businessTestApp(t)
	for i := 0; i < 7; i++ {
		row := playerRow(uint64(i+1), 1001, 500000)
		row.OwnerID = a.cfg.SystemOwner.IDBase
		r.systemCollectRows[a.cfg.AuctionDB] = append(r.systemCollectRows[a.cfg.AuctionDB], row)
	}
	mat := playerRow(90, 4000, 140000)
	mat.Count = 700
	mat.StartPrice = -1
	mat.OwnerID = a.cfg.SystemOwner.IDBase
	r.systemCollectRows[a.cfg.AuctionDB] = append(r.systemCollectRows[a.cfg.AuctionDB], mat, playerRow(99, 1001, 1))
	plan, err := a.Plan(RestockRequest{Market: "auction", ItemIDs: []uint32{1001, 4000}})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[uint32]int{}
	for _, act := range plan.Actions {
		counts[act.ItemID] += int(act.Count)
	}
	if counts[1001] != 3 || counts[4000] != 300 || len(plan.Actions) != 6 {
		t.Fatalf("缺口错误 %+v %+v", counts, plan)
	}
}

func TestBusinessExecutionReservesBeforeConcurrentProtocolCalls(t *testing.T) {
	a, r := businessTestApp(t)
	rules, status := a.businessSnapshot()
	status.Limits.DailyGold = 200000
	a.setPriceRangeState(rules, status)
	// 使用内存快照模拟热加载后稳定状态。
	a.runtimeFilesWatched.Store(true)
	catalog, _ := a.loadCatalog()
	var actions []Action
	for i := 0; i < 20; i++ {
		row := playerRow(uint64(i+1), 1001, 100000)
		r.collectRows[a.cfg.AuctionDB] = append(r.collectRows[a.cfg.AuctionDB], row)
		act, _, _, err := a.purchaseTerms(row, catalog[1001])
		if err != nil {
			t.Fatal(err)
		}
		actions = append(actions, act)
	}
	ok := true
	a.executors = fixedActionExecutorFactory{result: ActionExecutionResult{ResultOK: &ok}}
	_, entries, _ := a.executeActions(context.Background(), "test", actions, 8, true, &JobSummary{})
	success := 0
	for _, entry := range entries {
		if entry.OK {
			success++
		}
	}
	if success != 2 {
		t.Fatalf("预算被突破或未利用 success=%d %+v", success, entries)
	}
	l, _ := marketguard.Snapshot(a.configDir)
	if len(l.Trades) != 2 {
		t.Fatalf("记账数量=%d", len(l.Trades))
	}
	_, entries, _ = a.executeActions(context.Background(), "retry", actions, 8, true, &JobSummary{})
	for _, entry := range entries {
		if entry.OK {
			t.Fatal("重复成交")
		}
	}
}

func TestBusinessDisconnectKeepsReservationAndExplicitRejectReleases(t *testing.T) {
	for _, disconnect := range []bool{true, false} {
		t.Run(fmt.Sprint(disconnect), func(t *testing.T) {
			a, r := businessTestApp(t)
			row := playerRow(1, 1001, 80000)
			r.collectRows[a.cfg.AuctionDB] = []collectRow{row}
			plan, _ := a.CollectPlan(CollectRequest{Market: "auction"})
			if len(plan.Actions) != 1 {
				t.Fatal(plan)
			}
			no := false
			a.executors = fixedActionExecutorFactory{result: ActionExecutionResult{ResultOK: &no}}
			if disconnect {
				a.executors = fixedActionExecutorFactory{err: errors.New("断线")}
			}
			a.executeActions(context.Background(), "test", plan.Actions, 1, true, &JobSummary{})
			l, _ := marketguard.Snapshot(a.configDir)
			_, exists := l.Trades[a.tradeKey(1)]
			if exists != disconnect {
				t.Fatalf("预留=%v disconnect=%v", exists, disconnect)
			}
		})
	}
}

func TestBusinessConfigRejectsArbitrageAndReportsOldInventory(t *testing.T) {
	a, r := businessTestApp(t)
	status, err := a.BusinessStatus()
	if err != nil {
		t.Fatal(err)
	}
	doc := status.Config
	doc.Items[0].BuyMaxPrice = doc.Items[0].SellPrice
	if _, err := a.UpdateBusinessConfig(doc); err != nil {
		t.Fatalf("买卖同价应允许：%v", err)
	}
	doc.Items[0].BuyMaxPrice = doc.Items[0].SellPrice + 1
	if _, err := a.UpdateBusinessConfig(doc); err == nil {
		t.Fatal("接受收购价高于卖价")
	}
	doc.Items[0].BuyMaxPrice = 100000
	row := playerRow(1, 1001, 90000)
	row.OwnerID = a.cfg.SystemOwner.IDBase
	r.systemCollectRows[a.cfg.AuctionDB] = []collectRow{row}
	if _, err := a.UpdateBusinessConfig(doc); err == nil {
		t.Fatal("接受旧库存套利")
	}
	mustWriteText(t, appPaths(a).MarketPrices(), "{broken")
	a.refreshCustomPriceRanges()
	if _, err := a.Plan(RestockRequest{Market: "auction"}); err == nil {
		t.Fatal("损坏清单回落全目录补货")
	}
}
