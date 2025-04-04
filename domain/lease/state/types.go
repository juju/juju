// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql/driver"
	"fmt"
	"math"
	"time"
)

// Lease represents a lease to be serialised to the database.
type Lease struct {
	// UUID is the unique id of the lease.
	UUID string `db:"uuid"`
	// Type is the name of the type.
	Type string `db:"type"`
	// ModelUUID is the UUID of the model the lease is for.
	ModelUUID string `db:"model_uuid"`
	// Name is the name of the lease.
	Name string `db:"name"`
	// Holder is the holder of the lease.
	Holder string `db:"holder"`
	// Start is the lease start time.
	Start time.Time `db:"start"`
	// Duration is the duration of the lease. It is used only when inserting an
	// expiry time, the actual time is calculated from the duration by the
	// database.
	Duration LeaseDuration `db:"duration"`
	// Expiry is the lease expiry time. Expiry is only used to read expiry
	// times, Duration is used for writing.
	Expiry time.Time `db:"expiry"`
}

type LeaseDuration time.Duration

// Value implements the Valuer interface for scanning
func (d LeaseDuration) Value() (driver.Value, error) {
	return fmt.Sprintf("+%d seconds", int64(math.Ceil(time.Duration(d).Seconds()))), nil
}

// LeasePin represents a lease pin to be serialised to the database.
type LeasePin struct {
	// UUID is the unique identifier of the lease pin.
	UUID string `db:"uuid"`
	// EntityID is the id of the entity requesting the pin.
	EntityID string `db:"entity_id"`
}

// Count is used count leases.
type Count struct {
	Num int `db:"num"`
}

// leadership represents a single row from the leadership table for
// applications.
type leadership struct {
	ModelUUID string `db:"model_uuid"`
	Name      string `db:"name"`
	Holder    string `db:"holder"`
}
