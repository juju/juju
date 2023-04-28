// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import "github.com/juju/juju/core/arch"

// ArchOrDefault returns the arch for the constraint if there is one,
// else it returns the default arch.
func ArchOrDefault(cons Value, defaultCons *Value) string {
	if cons.HasArch() {
		return *cons.Arch
	}
	if defaultCons != nil && defaultCons.HasArch() {
		return *defaultCons.Arch
	}
	return arch.DefaultArchitecture
}
