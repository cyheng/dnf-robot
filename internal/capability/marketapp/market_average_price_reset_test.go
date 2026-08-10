package marketapp

import (
	"reflect"
	"testing"
)

func TestListingConfigRebuildClearDeletesOnlyAuctionAveragePrices(t *testing.T) {
	cfg := DefaultConfig()
	repo := &clearStockRepository{
		counts: map[string]int{cfg.AuctionDB: 3},
		averageCounts: map[string]int{
			cfg.AuctionDB: 7,
			cfg.CeraDB:    5,
		},
	}
	app := testApp(t)
	app.repository = repo

	result, err := app.clearSystemAuctionStockLocked("market_settings_apply")
	if err != nil {
		t.Fatal(err)
	}
	if result.AveragePriceDeleted != 7 {
		t.Fatalf("average price deleted = %d, want 7", result.AveragePriceDeleted)
	}
	if !reflect.DeepEqual(repo.averageDeletes, []string{cfg.AuctionDB}) {
		t.Fatalf("average price deletes = %v, want only auction", repo.averageDeletes)
	}
	if repo.averageCounts[cfg.CeraDB] != 5 {
		t.Fatalf("cera average prices changed during auction rebuild")
	}
}

func TestItemInfoReleaseDeletesAuctionAndCeraAveragePrices(t *testing.T) {
	cfg := DefaultConfig()
	repo := &clearStockRepository{
		counts: map[string]int{
			cfg.AuctionDB: 0,
			cfg.CeraDB:    0,
		},
		averageCounts: map[string]int{
			cfg.AuctionDB: 7,
			cfg.CeraDB:    5,
		},
	}
	app := testApp(t)
	app.repository = repo

	if err := app.prepareItemInfoRelease(); err != nil {
		t.Fatal(err)
	}
	want := []string{cfg.AuctionDB, cfg.CeraDB}
	if !reflect.DeepEqual(repo.averageDeletes, want) {
		t.Fatalf("average price deletes = %v, want %v", repo.averageDeletes, want)
	}
	if repo.averageCounts[cfg.AuctionDB] != 0 || repo.averageCounts[cfg.CeraDB] != 0 {
		t.Fatalf("average prices remain after iteminfo release: %v", repo.averageCounts)
	}
}
