package webadmin

import (
	"strings"
	"testing"
)

func TestBusinessDialogExposesSeparateBuySellAndSafeSave(t *testing.T) {
	for _, text := range []string{"marketBusinessStatus", "marketBusinessUpdate", "marketCollectPreview", "最高收购价", "每日金币", "sell_enabled", "buy_enabled", "target_quantity", "stack_size", "旧库存", "pending_trades", "market business", "if(saved)await openMarketBusinessDialog()"} {
		if !strings.Contains(appJS, text) {
			t.Errorf("经营管理缺少 %q", text)
		}
	}
	if strings.Contains(appJS, "marketOutRangeProbability") {
		t.Fatal("界面仍允许配置超价收购概率")
	}
}
