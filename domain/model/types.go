// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/errors"
	"github.com/juju/utils/v3"
)

// UUID represents a model unique identifier.
type UUID string

// Validate ensures the consistency of the UUID.
func (u UUID) Validate() error {
	if u == "" {
		return errors.New("empty uuid")
	}
	if !utils.IsValidUUIDString(string(u)) {
		return errors.Errorf("invalid uuid %q", u)
	}
	return nil
}

// String implements the stringer interface for UUID.
func (u UUID) String() string {
	return string(u)
}
