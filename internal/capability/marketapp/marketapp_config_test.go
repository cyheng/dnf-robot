package marketapp

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"robot/internal/foundation/config"
	"robot/internal/foundation/layout"
)

func TestNewRejectsEmptyConfigDir(t *testing.T) {
	if _, err := New(&sql.DB{}, &config.SysConfig{}, nil); err == nil || !strings.Contains(err.Error(), "config dir") {
		t.Fatalf("New error = %v, want empty config dir", err)
	}
}

func TestLoadConfigCreatesCommentedINIInConfDirectory(t *testing.T) {
	dir := t.TempDir()
	_, path, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != layout.New(dir).MarketConfig() {
		t.Fatalf("path=%q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "[auction_price]") || !strings.Contains(text, "category_price_rules = ") {
		t.Fatalf("generated INI lacks documented pricing configuration:\n%s", text)
	}
	if !strings.Contains(text, "equipment_allowed_rarities = 012345") || !strings.Contains(text, "other_allowed_rarities = 012345") || !strings.Contains(text, "equipment_trade_policy = permissive") || !strings.Contains(text, "other_trade_policy = permissive") {
		t.Fatalf("generated INI does not use the default listed rarity digits:\n%s", text)
	}
	if !strings.Contains(text, "blocked_item_ids = ") {
		t.Fatalf("generated INI lacks blocked item IDs setting:\n%s", text)
	}
	if !strings.Contains(text, "allowed_item_ids = ") {
		t.Fatalf("generated INI lacks allowed item IDs setting:\n%s", text)
	}
	for _, recommended := range []string{"category_price_rules = ", "equipment_multiplier_min = 1", "equipment_multiplier_max = 2", "equipment_final_max_price = 200000000"} {
		if !strings.Contains(text, recommended) {
			t.Fatalf("generated INI lacks recommended pricing setting %q:\n%s", recommended, text)
		}
	}
	if strings.Contains(text, "quality_filter =") || strings.Contains(text, "blocked_rarities =") {
		t.Fatalf("generated INI still contains legacy rarity settings:\n%s", text)
	}
	for _, unused := range []string{"listen_addr", "frida_db", "[service]", "auto_sync", "nexon_base", "recycle_price", "market_config.json", "custom_price_file", "source_path"} {
		if strings.Contains(text, unused) {
			t.Fatalf("generated INI still contains unused setting %q:\n%s", unused, text)
		}
	}
	if strings.Count(text, "max_result_actions =") != 1 || strings.Count(text, "per_item_delay_ms =") != 1 {
		t.Fatalf("collector duplicated restock-only action detail settings:\n%s", text)
	}
}

func TestLoadConfigNormalizesAllowedRaritiesAndAddsTradePolicies(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[auction_price]\nequipment_allowed_rarities = 43004\nother_allowed_rarities = 43004\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Restock.EquipmentAllowedRarities != "034" || cfg.Restock.OtherAllowedRarities != "034" {
		t.Fatalf("rarities=%q/%q, want 034/034", cfg.Restock.EquipmentAllowedRarities, cfg.Restock.OtherAllowedRarities)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "equipment_allowed_rarities = 034") || !strings.Contains(string(data), "other_allowed_rarities = 034") {
		t.Fatalf("normalized config does not contain canonical allowed rarities:\n%s", data)
	}
	if cfg.Restock.EquipmentTradePolicy != tradePolicyPermissive || cfg.Restock.OtherTradePolicy != tradePolicyPermissive {
		t.Fatalf("trade policies = %q/%q", cfg.Restock.EquipmentTradePolicy, cfg.Restock.OtherTradePolicy)
	}
}

func TestLoadConfigRejectsInvalidAllowedRarities(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[auction_price]\nequipment_allowed_rarities = 01a4\nother_allowed_rarities = 01234\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), "digits 0..9") {
		t.Fatalf("invalid allowed rarities error=%v", err)
	}
}

func TestMarketConfigRoundTripsBlockedItemIDs(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	cfg := DefaultConfig()
	cfg.Restock.BlockedItemIDs = []uint32{100, 300}
	cfg.Restock.AllowedItemIDs = []uint32{200, 400}
	cfg.Restock.CategoryPriceRules[valueCategoryBead] = PriceRule{MinPrice: 2500, MaxPrice: 90000, RarityWeight: .3, LevelWeight: .2, PVFWeight: .5}
	cfg.Restock.CategoryPriceRules[valueCategoryEquipment] = PriceRule{MinPrice: 5000, MaxPrice: 9000000, RarityWeight: .4, LevelWeight: .3, PVFWeight: .3}
	if err := writeMarketConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Restock.BlockedItemIDs; len(got) != 2 || got[0] != 100 || got[1] != 300 {
		t.Fatalf("blocked item IDs = %v, want [100 300]", got)
	}
	if got := loaded.Restock.AllowedItemIDs; len(got) != 2 || got[0] != 200 || got[1] != 400 {
		t.Fatalf("allowed item IDs = %v, want [200 400]", got)
	}
	if got := loaded.Restock.CategoryPriceRules[valueCategoryBead]; got.MinPrice != 2500 || got.MaxPrice != 90000 || got.PVFWeight != .5 {
		t.Fatalf("bead price rule did not round trip: %+v", got)
	}
	if got := loaded.Restock.CategoryPriceRules[valueCategoryEquipment]; got.MinPrice != 5000 || got.MaxPrice != 9000000 || got.LevelWeight != .3 {
		t.Fatalf("equipment price rule did not round trip: %+v", got)
	}
}

func TestMarketConfigRejectsUnknownPriceCategory(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Restock.CategoryPriceRules["unknown"] = PriceRule{MinPrice: 1, MaxPrice: 2, RarityWeight: 1}
	if err := validateMarketConfig(cfg); err == nil || !strings.Contains(err.Error(), "category_price_rules") {
		t.Fatalf("unknown value category error = %v", err)
	}
}

func TestBlockedItemIDRangesParseAndEncode(t *testing.T) {
	ids, err := decodeBlockedItemIDs("1, 2 3\n8-50，55,56")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 48 || ids[0] != 1 || ids[3] != 8 || ids[len(ids)-1] != 56 {
		t.Fatalf("decoded blocked IDs = %v", ids)
	}
	if got := encodeBlockedItemIDs(ids); got != "1-3,8-50,55-56" {
		t.Fatalf("encoded blocked IDs = %q", got)
	}
	for _, invalid := range []string{"0", "5-3", "1-2-3", "x", "1-100001"} {
		if _, err := decodeBlockedItemIDs(invalid); err == nil {
			t.Fatalf("invalid blocked expression %q was accepted", invalid)
		}
	}
}

func TestAllowedItemIDRangesParseAndEncode(t *testing.T) {
	ids, err := decodeAllowedItemIDs("1-3,8,9,10")
	if err != nil {
		t.Fatal(err)
	}
	if got := encodeAllowedItemIDs(ids); got != "1-3,8-10" {
		t.Fatalf("encoded allowed IDs = %q", got)
	}
	if _, err := decodeAllowedItemIDs("10-8"); err == nil || !strings.Contains(err.Error(), "allowed_item_ids") {
		t.Fatalf("invalid allowed expression error = %v", err)
	}
}

func TestLoadConfigRejectsInvalidCurrentINI(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken line without section"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfig(dir); err == nil {
		t.Fatal("invalid market INI unexpectedly loaded")
	}
}

func TestLoadConfigNormalizesINIAndKeepsExplicitSwitches(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	raw := `[auction_price]
equipment_level_min = 40
equipment_level_max = 70
category_price_rules = {"equipment":{"min_price":2500,"max_price":9000000,"rarity_weight":0.4,"level_weight":0.3,"pvf_weight":0.3}}
equipment_multiplier_min = 1.25
equipment_multiplier_max = 2.5
equipment_final_max_price = 200000000
upgrade_min = 6
upgrade_max = 11
upgrade_price_rate = 0.12
rand_low = 0.75
rand_high = 1.25
custom_price_enabled = true

[auction_collect]
enabled = false
price_range_enabled = true
in_range_probability = 0.9
out_of_range_probability = 0.02
`
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Restock.EquipmentLevelMin != 40 || cfg.Restock.EquipmentLevelMax != 70 || cfg.Restock.CategoryPriceRules[valueCategoryEquipment].MinPrice != 2500 || cfg.Restock.CategoryPriceRules[valueCategoryEquipment].MaxPrice != 9000000 || cfg.Restock.EquipmentMultiplierMin != 1.25 || cfg.Restock.EquipmentMultiplierMax != 2.5 || cfg.Restock.UpgradePriceRate != 0.12 || !cfg.Restock.CustomPriceEnabled {
		t.Fatalf("pricing config=%+v", cfg.Restock)
	}
	if cfg.Collector.Enabled || !cfg.Collector.PriceRangeEnabled || cfg.Collector.InRangeProbability != 0.9 || cfg.Collector.OutRangeProbability != 0.02 {
		t.Fatalf("collector config=%+v", cfg.Collector)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, comment := range []string{
		"# 单轮补货最多生成并执行的动作数；0 表示配置层不限制。",
		"# 补货动作的最大并发工作数。",
		"# 单个任务结果中最多保留的动作明细数，避免接口和日志数据过大。",
		"# 同一工作线程连续执行补货或回收动作时的间隔毫秒数；0 表示不主动等待。",
		"# 允许上架的最低装备等级；0 表示不限制最低等级。",
		"# 允许上架的最高装备等级；0 表示不限制最高等级。",
	} {
		if !strings.Contains(text, comment) {
			t.Fatalf("normalized INI lacks action-limit comment %q:\n%s", comment, text)
		}
	}
}

func TestLoadConfigRejectsRemovedDynamicPaths(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{
		"[iteminfo]\nsource_path = ../pvf/iteminfo.dat\n",
		"[auction_price]\ncustom_price_file = prices.json\n",
	} {
		if err := os.WriteFile(path, []byte(setting), 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadConfig(dir); err == nil {
			t.Fatalf("removed dynamic path unexpectedly accepted: %s", setting)
		}
	}
}

func TestLoadConfigRejectsUnknownAndDuplicateSettings(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"[unknown]\nvalue = 1\n",
		"[auto]\nenabeld = true\n",
		"[auto]\nenabled = true\nenabled = false\n",
		"[auction_price]\nvalue_category_recognition = material|10\n",
		"[auction_price]\nequip_inflate_min = 1\n",
	} {
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadConfig(dir); err == nil {
			t.Fatalf("invalid market setting unexpectedly accepted: %s", raw)
		}
	}
}

func TestLoadConfigRejectsNonCanonicalCase(t *testing.T) {
	dir := t.TempDir()
	path := layout.New(dir).MarketConfig()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"[DATABASE]\ngame_db = custom_game\n",
		"[database]\nGAME_DB = custom_game\n",
	} {
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadConfig(dir); err == nil || !strings.Contains(err.Error(), "canonical lowercase") {
			t.Fatalf("non-canonical market setting error = %v, raw=%q", err, raw)
		}
	}
}
