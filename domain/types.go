// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// NullableDuration represents a nullable time.Duration.
type NullableDuration struct {
	Duration time.Duration
	Valid    bool
}

// Scan implements the sql.Scanner interface.
func (nd *NullableDuration) Scan(value interface{}) error {
	if value == nil {
		nd.Duration, nd.Valid = 0, false
		return nil
	}
	switch v := value.(type) {
	case int64:
		nd.Duration = time.Duration(v)
		nd.Valid = true
	default:
		return fmt.Errorf("cannot scan type %T into NullableDuration", value)
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (nd NullableDuration) Value() (driver.Value, error) {
	if !nd.Valid {
		return nil, nil
	}
	return int64(nd.Duration), nil
}
