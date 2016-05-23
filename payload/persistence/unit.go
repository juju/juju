// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package persistence

import (
	"github.com/juju/juju/payload"
)

// UnitPersistence exposes the high-level persistence functionality
// related to payloads in Juju.
type UnitPersistence struct {
	pp   *Persistence
	unit string
}

// NewUnitPersistence builds a new Persistence based on the provided info.
func NewUnitPersistence(pp *Persistence, unit string) *UnitPersistence {
	return &UnitPersistence{
		pp:   pp,
		unit: unit,
	}
}

// Track adds records for the payload to persistence. If the payload
// is already there then false gets returned (true if inserted).
// Existing records are not checked for consistency.
func (up UnitPersistence) Track(pl payload.FullPayloadInfo) error {
	return up.pp.Track(pl)
}

// SetStatus updates the raw status for the identified payload in
// persistence. The return value corresponds to whether or not the
// record was found in persistence. Any other problem results in
// an error. The payload is not checked for inconsistent records.
func (up UnitPersistence) SetStatus(name, status string) error {
	return up.pp.SetStatus(up.unit, name, status)
}

// List builds the list of payloads found in persistence which match
// the provided IDs. The lists of IDs with missing records is also
// returned.
func (up UnitPersistence) List(names ...string) ([]payload.FullPayloadInfo, []string, error) {
	return up.pp.List(up.unit, names...)
}

// ListAll builds the list of all payloads found in persistence.
// Inconsistent records result in errors.NotValid.
func (up UnitPersistence) ListAll() ([]payload.FullPayloadInfo, error) {
	return up.pp.ListAllForUnit(up.unit)
}

// Untrack removes all records associated with the identified payload
// from persistence. Also returned is whether or not the payload was
// found. If the records for the payload are not consistent then
// errors.NotValid is returned.
func (up UnitPersistence) Untrack(name string) error {
	return up.pp.Untrack(up.unit, name)
}
