// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"time"
)

// NullDuration represents a nullable time.Duration.
type NullDuration struct {
	Duration time.Duration
	Valid    bool
}

// NewNullDuration returns a new NullDuration with the given duration.
func NewNullDuration(d time.Duration) NullDuration {
	return NullDuration{Duration: d, Valid: true}
}

// Scan implements the sql.Scanner interface.
func (nd *NullDuration) Scan(value interface{}) error {
	if value == nil {
		nd.Duration, nd.Valid = 0, false
		return nil
	}
	switch v := value.(type) {
	case int64:
		nd.Duration = time.Duration(v)
		nd.Valid = true
	default:
		return fmt.Errorf("cannot scan type %T into NullDuration", value)
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (nd NullDuration) Value() (driver.Value, error) {
	if !nd.Valid {
		return nil, nil
	}
	return int64(nd.Duration), nil
}

// Uint64 represents a uint64 value.
// The sql driver only handles signed ints.
type Uint64 struct {
	UnsignedValue uint64
	Valid         bool
}

// NewUint64 returns a Uint64 with the given value.
func NewUint64(v uint64) Uint64 {
	return Uint64{UnsignedValue: v, Valid: true}
}

// Scan implements the sql.Scanner interface.
func (ui *Uint64) Scan(value interface{}) error {
	if value == nil {
		ui.UnsignedValue, ui.Valid = 0, false
		return nil
	}

	switch v := value.(type) {
	case []byte:
		ui.UnsignedValue = binary.NativeEndian.Uint64(v)
		ui.Valid = len(v) == 8
	default:
		return fmt.Errorf("cannot scan type %T into uint64", value)
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (ui Uint64) Value() (driver.Value, error) {
	if !ui.Valid {
		return nil, nil
	}
	v := make([]byte, 8)
	binary.NativeEndian.PutUint64(v, ui.UnsignedValue)
	return v, nil
}
