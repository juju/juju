// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"database/sql/driver"
	"time"

	"github.com/juju/juju/internal/errors"
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
		return errors.Errorf("cannot scan type %T into NullDuration", value)
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

// NullStringTime represents a [time.Time] that may be null.
type NullStringTime struct {
	Time  time.Time
	Valid bool // Valid is true if Time is not NULL
}

// Scan implements the [Scanner] interface.
func (n *NullStringTime) Scan(value any) error {
	if value == nil {
		n.Time, n.Valid = time.Time{}, false
		return nil
	}
	n.Valid = true
	switch v := value.(type) {
	case string:
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return errors.Errorf("parsing time from string: %w", err)
		}
		n.Time = t
		return nil
	case *string:
		t, err := time.Parse(time.RFC3339Nano, *v)
		if err != nil {
			return errors.Errorf("parsing time from *string: %w", err)
		}
		n.Time = t
		return nil
	default:
		return errors.Errorf("cannot scan type %T into NullStringTime", value)
	}
}

// Value implements the [driver.Valuer] interface.
func (n NullStringTime) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return FormatStringTime(n.Time), nil
}

// StringTime represents a [time.Time] that may be null.
type StringTime struct {
	Time time.Time
}

// Scan implements the [Scanner] interface.
func (n *StringTime) Scan(value any) error {
	if value == nil {
		return errors.Errorf("cannot scan NULL into StringTime")
	}
	switch v := value.(type) {
	case string:
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			return errors.Errorf("parsing time from string: %w", err)
		}
		n.Time = t
		return nil
	default:
		return errors.Errorf("cannot scan type %T into StringTime", value)
	}
}

// Value implements the [driver.Valuer] interface.
func (n StringTime) Value() (driver.Value, error) {
	return FormatStringTime(n.Time), nil
}

// FormatStringTime formats the time as a UTC RFC3339Nano string.
func FormatStringTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// ParseStringTime parses the time from a UTC RFC3339Nano string.
func ParseStringTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

type NullBlob struct {
	Data  string
	Valid bool
}

// Scan implements the [Scanner] interface.
func (n *NullBlob) Scan(value any) error {
	if value == nil {
		n.Data, n.Valid = "", false
		return nil
	}
	n.Valid = true
	switch v := value.(type) {
	case []byte:
		n.Data = string(v)
		return nil
	case *[]byte:
		n.Data = string(*v)
		return nil
	default:
		return errors.Errorf("cannot scan type %T into Blob", value)
	}
}

// Value implements the [driver.Valuer] interface.
func (n NullBlob) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return []byte(n.Data), nil
}

type Blob struct {
	Data string
}

// Scan implements the [Scanner] interface.
func (n *Blob) Scan(value any) error {
	if value == nil {
		return errors.Errorf("cannot scan NULL into Blob")
	}
	switch v := value.(type) {
	case []byte:
		n.Data = string(v)
		return nil
	default:
		return errors.Errorf("cannot scan type %T into Blob", value)
	}
}

// Value implements the [driver.Valuer] interface.
func (n Blob) Value() (driver.Value, error) {
	return []byte(n.Data), nil
}
