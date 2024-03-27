// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql/driver"
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
