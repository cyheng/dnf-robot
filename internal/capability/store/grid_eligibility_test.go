package store

import (
	"testing"

	"robot/internal/shared"
)

func eligibility(value bool) *bool { return &value }

func TestGateAreaEligibilityDiffersForNormalAndStoreRoles(t *testing.T) {
	mp := shared.MapCatalogItem{Use: true, Gate: true, NormalEligible: eligibility(true), StoreEligible: eligibility(false)}
	if !IsNormalMapEligible(mp) {
		t.Fatal("gate area must be eligible for normal robots")
	}
	if IsStoreMapEligible(mp) {
		t.Fatal("gate area must not be eligible for private stores")
	}
}

func TestStoreCoordinatorValidatesTargetFromCurrentCatalog(t *testing.T) {
	configDir := t.TempDir()
	writeStoreMapCatalog(t, configDir, []shared.MapCatalogItem{
		{Village: 1, Area: 0, Use: true, StoreEligible: eligibility(true), Rectangles: []shared.MapRectangle{{XMin: 1, XMax: 200, YMin: 1, YMax: 200}}},
		{Village: 1, Area: 1, Use: true, Gate: true, StoreEligible: eligibility(false), Rectangles: []shared.MapRectangle{{XMin: 1, XMax: 200, YMin: 1, YMax: 200}}},
	})
	coordinator := newTestPointCoordinator(configDir, nil)
	if !coordinator.HasArea(1, 0) {
		t.Fatal("current PVF store area was not available as a migration target")
	}
	if coordinator.HasArea(1, 1) {
		t.Fatal("gate area became a store migration target")
	}
}

func TestBuildGridPointsRejectsCatalogGateMetadata(t *testing.T) {
	points := BuildGridPoints([]shared.MapCatalogItem{{
		Village: 3, Area: 0, Use: true, Gate: true,
		Rectangles: []shared.MapRectangle{{XMin: 1, XMax: 200, YMin: 1, YMax: 200}},
	}})
	if len(points) != 0 {
		t.Fatalf("gate points=%+v, want none", points)
	}
}

func TestBuildGridPointsMarksOtherAreasAsProbeOnly(t *testing.T) {
	points := BuildGridPoints([]shared.MapCatalogItem{
		{Village: 2, Area: 0, Use: true, StoreEligible: eligibility(true), StoreProbe: eligibility(false), Rectangles: []shared.MapRectangle{{XMin: 1, XMax: 1, YMin: 1, YMax: 1}}},
		{Village: 2, Area: 1, Use: true, StoreEligible: eligibility(false), StoreProbe: eligibility(true), Rectangles: []shared.MapRectangle{{XMin: 1, XMax: 1, YMin: 1, YMax: 1}}},
		{Village: 2, Area: 2, Use: true, StoreEligible: eligibility(false), StoreProbe: eligibility(false), Rectangles: []shared.MapRectangle{{XMin: 1, XMax: 1, YMin: 1, YMax: 1}}},
	})
	if len(points) != 2 {
		t.Fatalf("points=%+v, want normal and probe points", points)
	}
	if points[0].Probe || !points[1].Probe {
		t.Fatalf("probe classification=%+v", points)
	}
}

func TestFilterEligibleGridPointsPreservesPromotedProbePoint(t *testing.T) {
	points := []GridPoint{{ID: "2-1-1-1", Village: 2, Area: 1, X: 1, Y: 1, Status: PointStatusSuccess, Success: 1, ProbeVerified: true}}
	maps := []shared.MapCatalogItem{{Village: 2, Area: 1, Use: true, StoreEligible: eligibility(false), StoreProbe: eligibility(true)}}
	got := FilterEligibleGridPoints(points, maps)
	if len(got) != 1 || got[0].Probe {
		t.Fatalf("promoted probe point was reverted: %+v", got)
	}
}

func TestFilterEligibleGridPointsReprobesUnverifiedPromotion(t *testing.T) {
	points := []GridPoint{{
		ID: "2-1-1-1", Village: 2, Area: 1, X: 1, Y: 1,
		Status: PointStatusSuccess, Success: 3, LastUID: 1001, LastReason: StoreReasonAck,
	}}
	maps := []shared.MapCatalogItem{{Village: 2, Area: 1, Use: true, StoreEligible: eligibility(false), StoreProbe: eligibility(true)}}
	got := FilterEligibleGridPoints(points, maps)
	if len(got) != 1 || !got[0].Probe || got[0].Status != PointStatusUnknown || got[0].Success != 0 || got[0].LastReason != "" {
		t.Fatalf("unverified promotion was not reset for probing: %+v", got)
	}
}

func TestFilterNormalMapsKeepsSafeAreasAndGatesOnly(t *testing.T) {
	maps := []shared.MapCatalogItem{
		{Village: 1, Area: 0, Use: true, NormalEligible: eligibility(true)},
		{Village: 1, Area: 1, Use: true, Gate: true, NormalEligible: eligibility(true)},
		{Village: 3, Area: 1, Use: true, NormalEligible: eligibility(false)},
		{Village: 2, Area: 0, Use: false},
	}
	got := FilterNormalMaps(maps)
	if len(got) != 2 || got[0].Area != 0 || !got[1].Gate {
		t.Fatalf("normal maps=%+v, want safe area and usable gate only", got)
	}
}
