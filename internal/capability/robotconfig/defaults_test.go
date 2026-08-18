package robotconfig

import "testing"

func TestNormalizeKeepsPVFDefinedVillageAboveLegacyRange(t *testing.T) {
	rc := Default()
	rc.SpawnVillage = 26
	Normalize(&rc)
	if rc.SpawnVillage != 26 {
		t.Fatalf("spawn village=%d, want 26", rc.SpawnVillage)
	}
}

func TestNormalizeStoreEquipmentIntensifyRange(t *testing.T) {
	rc := Default()
	rc.StoreEquipmentIntensifyMin = 40
	rc.StoreEquipmentIntensifyMax = 6
	Normalize(&rc)
	if rc.StoreEquipmentIntensifyMin != 6 || rc.StoreEquipmentIntensifyMax != 31 {
		t.Fatalf("store equipment intensify=%d..%d, want 6..31", rc.StoreEquipmentIntensifyMin, rc.StoreEquipmentIntensifyMax)
	}
}

func TestNormalizeStoreEquipmentPriceWeights(t *testing.T) {
	rc := Default()
	rc.StoreEquipmentLevelWeight = -1
	rc.StoreEquipmentRarityWeight = 8
	rc.StoreEquipmentIntensifyWeight = 5
	Normalize(&rc)
	if rc.StoreEquipmentLevelWeight != 0 || rc.StoreEquipmentRarityWeight != 8 || rc.StoreEquipmentIntensifyWeight != 5 {
		t.Fatalf("store equipment weights=%d/%d/%d", rc.StoreEquipmentLevelWeight, rc.StoreEquipmentRarityWeight, rc.StoreEquipmentIntensifyWeight)
	}
	rc.StoreEquipmentRarityWeight = 0
	rc.StoreEquipmentIntensifyWeight = 0
	Normalize(&rc)
	if rc.StoreEquipmentLevelWeight != 35 || rc.StoreEquipmentRarityWeight != 40 || rc.StoreEquipmentIntensifyWeight != 25 {
		t.Fatalf("default store equipment weights=%d/%d/%d", rc.StoreEquipmentLevelWeight, rc.StoreEquipmentRarityWeight, rc.StoreEquipmentIntensifyWeight)
	}
}
