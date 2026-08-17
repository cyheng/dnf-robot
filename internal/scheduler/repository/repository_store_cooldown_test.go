package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestStorePermissionCooldownOnlyForCrashedTable(t *testing.T) {
	r := &SQLRepository{}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	r.recordStorePermissionFailure(errors.New("ordinary failure"), now)
	if err := r.storePermissionCooldownError(now); err != nil {
		t.Fatalf("ordinary error started cooldown: %v", err)
	}
	r.recordStorePermissionFailure(&mysql.MySQLError{Number: 145, Message: "table is marked as crashed"}, now)
	if err := r.storePermissionCooldownError(now.Add(time.Second)); err == nil {
		t.Fatal("crashed-table error did not start cooldown")
	}
	if err := r.storePermissionCooldownError(now.Add(storePermissionDatabaseCooldown)); err != nil {
		t.Fatalf("cooldown did not expire: %v", err)
	}
}
