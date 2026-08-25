package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	robotconfig "robot/internal/capability/robotconfig"
	"robot/internal/shared"
)

type petTestRepository struct {
	missingSchemaRepository
	err       error
	calls     int
	pet       shared.EquipmentCatalogItem
	artifacts map[int]shared.EquipmentCatalogItem
}

func (r *petTestRepository) Stats() sql.DBStats { return sql.DBStats{} }

func (r *petTestRepository) PingContext(context.Context) error { return nil }

func (r *petTestRepository) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return &sql.Row{}
}

func (r *petTestRepository) ReplacePetItems(_ int, pet shared.EquipmentCatalogItem, artifacts map[int]shared.EquipmentCatalogItem) error {
	r.calls++
	r.pet = pet
	r.artifacts = artifacts
	return r.err
}

func TestPetProbabilityHitBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		percent int
		roll    int
		want    bool
	}{
		{name: "zero never hits", percent: 0, roll: 0, want: false},
		{name: "hundred always hits", percent: 100, roll: 99, want: true},
		{name: "below threshold hits", percent: 80, roll: 79, want: true},
		{name: "threshold misses", percent: 80, roll: 80, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := petProbabilityHit(test.percent, func(int) int { return test.roll }); got != test.want {
				t.Fatalf("petProbabilityHit(%d, %d) = %t, want %t", test.percent, test.roll, got, test.want)
			}
		})
	}
}

func TestPetFromCatalogSkipsOptionalPetWhenProbabilityMisses(t *testing.T) {
	repo := &petTestRepository{}
	manager := NewRobotManager(repo, nil, nil)
	err := manager.petFromCatalog(661, robotconfig.RuntimeConfig{PetEnabled: true, PetProbabilityPercent: 0}, []shared.EquipmentCatalogItem{{ID: 63050, ItemType: 30}})
	if err != nil || repo.calls != 0 {
		t.Fatalf("petFromCatalog error=%v writes=%d, want optional skip", err, repo.calls)
	}
}

func TestPetFromCatalogMissingCreatureDoesNotFailCreation(t *testing.T) {
	manager := NewRobotManager(nil, nil, nil)
	err := manager.petFromCatalog(661, robotconfig.RuntimeConfig{PetEnabled: true, PetProbabilityPercent: 100}, nil)
	if err != nil {
		t.Fatalf("petFromCatalog error=%v, want optional skip", err)
	}
}

func TestPetFromCatalogKeepsPetWhenArtifactsAreUnavailable(t *testing.T) {
	repo := &petTestRepository{}
	manager := NewRobotManager(repo, nil, nil)
	rc := robotconfig.RuntimeConfig{
		PetEnabled:            true,
		PetProbabilityPercent: 100,
		PetArtifactEnabled:    true,
		PetArtifactSlots:      []int{31, 32, 33},
		MinPetArtifactSlots:   1,
		MaxPetArtifactSlots:   2,
	}
	err := manager.petFromCatalog(661, rc, []shared.EquipmentCatalogItem{{ID: 63050, ItemType: 30}})
	if err != nil || repo.calls != 1 || repo.pet.ID != 63050 || len(repo.artifacts) != 0 {
		t.Fatalf("petFromCatalog error=%v writes=%d pet=%d artifacts=%v", err, repo.calls, repo.pet.ID, repo.artifacts)
	}
}

func TestPetFromCatalogWriteFailureDoesNotFailCreation(t *testing.T) {
	repo := &petTestRepository{err: errors.New("write failed")}
	manager := NewRobotManager(repo, nil, nil)
	err := manager.petFromCatalog(661, robotconfig.RuntimeConfig{PetEnabled: true, PetProbabilityPercent: 100}, []shared.EquipmentCatalogItem{{ID: 63050, ItemType: 30}})
	if err != nil || repo.calls != 1 {
		t.Fatalf("petFromCatalog error=%v writes=%d, want optional write failure", err, repo.calls)
	}
}
