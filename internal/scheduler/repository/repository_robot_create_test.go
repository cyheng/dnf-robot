package repository

import (
	"encoding/binary"
	"reflect"
	"strings"
	"testing"

	robotcap "robot/internal/capability/robot"
	"robot/internal/shared"
)

func TestCreateCharacterStatInsertInitializesPreviousVillage(t *testing.T) {
	query, args := createCharacterStatInsert(661, 12345, 5)
	if !strings.Contains(query, "village,village_prev") {
		t.Fatalf("query does not initialize both village fields: %s", query)
	}
	want := []interface{}{661, "100", 12345, "-1", 5, 5}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestPetInventoryUpdateOnlyTargetsCreatureColumns(t *testing.T) {
	query, args := petInventoryUpdate(661, shared.EquipmentCatalogItem{ID: 63050}, map[int]shared.EquipmentCatalogItem{
		31: {ID: 63500, ItemType: 31},
	}, true)
	if query != "UPDATE taiwan_cain_2nd.inventory SET creature=?,creature_flag=1 WHERE charac_no=?" {
		t.Fatalf("pet inventory query=%q", query)
	}
	if len(args) != 2 || args[1] != 661 {
		t.Fatalf("pet inventory args=%#v", args)
	}
	blob, ok := args[0].([]byte)
	if !ok || len(blob) < 4 || binary.LittleEndian.Uint32(blob[:4]) != 102*61 {
		t.Fatalf("pet creature blob header=%#v", args[0])
	}
}

func TestPetCreatureItemInsertSelectsCurrentSchemaWithoutFallback(t *testing.T) {
	columns := map[string]bool{
		"no_charge": true, "stat": true, "item_lock_key": true,
		"ipg_agency_no": true, "expire_date": true, "delete_date": true,
	}
	query, args, err := petCreatureItemInsert(661, 63050, columns)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "no_charge,stat,item_lock_key") || strings.Contains(query, "creature_level") {
		t.Fatalf("current creature insert query=%q", query)
	}
	if !reflect.DeepEqual(args, []interface{}{661, 63050}) {
		t.Fatalf("current creature insert args=%#v", args)
	}
}

func TestPetCreatureItemInsertRejectsUnknownSchema(t *testing.T) {
	if _, _, err := petCreatureItemInsert(661, 63050, map[string]bool{}); err == nil {
		t.Fatal("unknown creature_items schema was accepted")
	}
}

func TestCreateCharacterInfoInsertSuppliesStrictSchemaFields(t *testing.T) {
	info := robotcap.Info{UID: 17000508, CID: 31, Village: 1, Level: 70, Job: 0, Grow: 0}
	query, args := createCharacterInfoInsert(info, 12345, "robot", map[string]bool{
		"element_resist": true,
		"spec_property":  true,
		"VIP":            true,
		"create_time":    true,
	})
	for _, field := range []string{"element_resist", "spec_property", "VIP", "create_time"} {
		if !strings.Contains(query, "`"+field+"`") {
			t.Fatalf("query does not include %s: %s", field, query)
		}
	}
	if got := strings.Count(query, "?"); got != len(args) {
		t.Fatalf("placeholder count=%d args=%d", got, len(args))
	}
	if got, ok := args[24].([]byte); !ok || len(got) != 8 {
		t.Fatalf("element_resist arg=%#v", args[24])
	}
	if got, ok := args[25].([]byte); !ok || len(got) != 34 {
		t.Fatalf("spec_property arg=%#v", args[25])
	}
}

func TestCreateCharacterInfoInsertOmitsUnavailableStrictFields(t *testing.T) {
	info := robotcap.Info{UID: 17000508, CID: 31, Village: 1, Level: 70}
	query, args := createCharacterInfoInsert(info, 12345, "robot", map[string]bool{})
	for _, field := range []string{"element_resist", "spec_property", "VIP", "create_time"} {
		if strings.Contains(query, "`"+field+"`") {
			t.Fatalf("query unexpectedly includes %s: %s", field, query)
		}
	}
	if got := strings.Count(query, "?"); got != len(args) {
		t.Fatalf("placeholder count=%d args=%d", got, len(args))
	}
}
