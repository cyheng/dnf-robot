package marketapp

import (
	"context"
	"errors"
	"fmt"

	"robot/internal/foundation/marketguard"
)

var ErrPurchasePending = errors.New("成交状态待核对，保留预算和数量")

func (a *App) observeSystemSalePrices(catalog map[uint32]catalogItem) error {
	cfg := a.configSnapshot()
	rows, err := a.repository.LoadSystemCollectRows(cfg.AuctionDB, marketNameAuction, cfg.SystemOwner.IDBase)
	if err != nil {
		return err
	}
	prices := map[uint32]int32{}
	for _, row := range rows {
		if row.OwnerID < cfg.SystemOwner.IDBase || row.InstantPrice <= 0 {
			continue
		}
		item, known := catalog[row.ItemID]
		if !known {
			continue
		}
		count, valid := listingQuantity(row, item)
		if !valid {
			return fmt.Errorf("无法核对旧系统库存 item=%d", row.ItemID)
		}
		price := row.InstantPrice / count
		if price <= 0 {
			return fmt.Errorf("旧系统库存单价异常 item=%d", row.ItemID)
		}
		if old := prices[row.ItemID]; old == 0 || price < old {
			prices[row.ItemID] = price
		}
	}
	// 现有摊位可能在升级程序前生成，首次收购前必须一并记录。
	if repo, ok := a.repository.(interface {
		LoadStoreSalePrices() (map[uint32]int32, error)
	}); ok {
		storePrices, err := repo.LoadStoreSalePrices()
		if err != nil {
			return err
		}
		for id, price := range storePrices {
			if old := prices[id]; old == 0 || price < old {
				prices[id] = price
			}
		}
	} else {
		return fmt.Errorf("仓库不支持机器人摊位价格核对，停止收购")
	}
	return marketguard.ObserveSales(a.configDir, prices)
}

func (r SQLRepository) LoadStoreSalePrices() (map[uint32]int32, error) {
	rows, err := r.db.Query("SELECT Trade_item,MIN(price) FROM d_starsky.Robot_stall WHERE function_type=2 GROUP BY Trade_item")
	if err != nil {
		if isMissingTable(err) {
			return map[uint32]int32{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	prices := map[uint32]int32{}
	for rows.Next() {
		var id uint32
		var price int32
		if err := rows.Scan(&id, &price); err != nil {
			return nil, err
		}
		prices[id] = price
	}
	return prices, rows.Err()
}

type purchaseGuard struct {
	app     *App
	catalog map[uint32]catalogItem
	rows    map[uint64]collectRow
	err     error
}

func (a *App) newPurchaseGuard(actions []Action) purchaseGuard {
	g := purchaseGuard{app: a, rows: map[uint64]collectRow{}}
	needed := false
	for _, action := range actions {
		if action.Market == marketNameAuction && action.Operation == "collect" {
			needed = true
			break
		}
	}
	if !needed {
		return g
	}
	a.refreshCustomPriceRanges()
	g.catalog, g.err = a.loadCatalog()
	if g.err != nil {
		return g
	}
	if g.err = a.observeSystemSalePrices(g.catalog); g.err != nil {
		return g
	}
	cfg := a.configSnapshot()
	var rows []collectRow
	rows, g.err = a.repository.LoadCollectRows(cfg.AuctionDB, marketNameAuction, cfg.SystemOwner.IDBase, true)
	for _, row := range rows {
		g.rows[row.AuctionID] = row
	}
	return g
}

func (g purchaseGuard) execute(executor ActionExecutor, ctx context.Context, action Action) (ActionExecutionResult, error) {
	a := g.app
	if action.Market != marketNameAuction {
		return executeActionSafely(executor, ctx, action)
	}
	if action.Operation != "collect" {
		if err := marketguard.RecordSales(a.configDir, map[uint32]int32{action.ItemID: action.UnitPrice}); err != nil {
			return ActionExecutionResult{}, err
		}
		return executeActionSafely(executor, ctx, action)
	}
	if g.err != nil {
		return ActionExecutionResult{}, g.err
	}
	row, exists := g.rows[action.AuctionID]
	if !exists {
		return ActionExecutionResult{}, fmt.Errorf("挂单已不存在，停止购买")
	}
	if action.SystemCleanup {
		if row.OwnerID < a.configSnapshot().SystemOwner.IDBase {
			return ActionExecutionResult{}, fmt.Errorf("内部清理只能处理系统订单")
		}
		return executeActionSafely(executor, ctx, action)
	}
	canonical, trade, limits, err := a.purchaseTerms(row, g.catalog[row.ItemID])
	if err != nil {
		return ActionExecutionResult{}, err
	}
	if canonical.ItemID != action.ItemID || canonical.TotalPrice != action.TotalPrice || canonical.Count != action.Count || canonical.InstantPrice != action.InstantPrice {
		return ActionExecutionResult{}, fmt.Errorf("挂单发生变化，需重新规划")
	}
	if err = ctx.Err(); err != nil {
		return ActionExecutionResult{}, err
	}
	key := a.tradeKey(action.AuctionID)
	if err = marketguard.Reserve(a.configDir, key, trade, limits); err != nil {
		return ActionExecutionResult{}, err
	}
	res, err := executeActionSafely(executor, ctx, canonical)
	if errors.Is(err, ErrActionNotSubmitted) || errors.Is(err, ErrExecutorUnavailable) {
		if finishErr := marketguard.Finish(a.configDir, key, false); finishErr != nil {
			return res, fmt.Errorf("%w：%v", ErrPurchasePending, finishErr)
		}
		return res, err
	}
	if err == nil && res.ResultOK != nil {
		if finishErr := marketguard.Finish(a.configDir, key, *res.ResultOK); finishErr != nil {
			return res, fmt.Errorf("%w：协议已返回，记账失败：%v", ErrPurchasePending, finishErr)
		}
	} else {
		// 无法确认失败时不得释放预算或再次购买；需核对服务成交记录。
		return res, fmt.Errorf("%w：%v", ErrPurchasePending, err)
	}
	return res, err
}
