package scheduler

import (
	"strings"
	"testing"

	robotconfig "robot/internal/capability/robotconfig"
	"robot/internal/shared"
)

func TestPetFromCatalogRequiresCompatibleCreatureWhenEnabled(t *testing.T) {
	manager := NewRobotManager(nil, nil, nil)
	err := manager.petFromCatalog(661, robotconfig.RuntimeConfig{PetEnabled: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "no compatible creature") {
		t.Fatalf("petFromCatalog error=%v, want missing creature error", err)
	}
}

func TestPetFromCatalogEnforcesArtifactMinimum(t *testing.T) {
	manager := NewRobotManager(nil, nil, nil)
	rc := robotconfig.RuntimeConfig{
		PetEnabled:          true,
		PetArtifactEnabled:  true,
		PetArtifactSlots:    []int{31, 32, 33},
		MinPetArtifactSlots: 1,
		MaxPetArtifactSlots: 2,
	}
	err := manager.petFromCatalog(661, rc, []shared.EquipmentCatalogItem{{ID: 63050, ItemType: 30}})
	if err == nil || !strings.Contains(err.Error(), "below configured minimum 1") {
		t.Fatalf("petFromCatalog error=%v, want artifact minimum error", err)
	}
}
