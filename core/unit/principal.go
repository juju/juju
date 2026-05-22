// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit

// NameWithPrincipal pairs a unit name with its optional principal unit name.
// When Principal is nil the unit is a principal unit itself; when Principal is
// non-nil the unit is a subordinate and Principal holds the name of the
// principal unit it is deployed alongside.
type NameWithPrincipal struct {
	// Name is the name of the unit.
	Name Name

	// Principal is the name of the principal unit this unit is subordinate to.
	// It is nil when the unit is not a subordinate.
	Principal *Name
}

// IsSubordinate reports whether the unit is a subordinate (i.e. it has a
// principal unit).
func (u NameWithPrincipal) IsSubordinate() bool {
	return u.Principal != nil
}
