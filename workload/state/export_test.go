// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

var (
	NewID = newID
)

func SetNewID(uw UnitWorkloads, newID func() (string, error)) UnitWorkloads {
	uw.newID = newID
	return uw
}
