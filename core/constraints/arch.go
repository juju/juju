// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

// ArchOrDefault returns the arch for the constraint if there is one,
// falling back to the arch from the default constraint if there is one
func ArchOrDefault(cons Value, defaultCons Value) string {
	if cons.HasArch() {
		return *cons.Arch
	}
	if defaultCons.HasArch() {
		return *defaultCons.Arch
	}
	return ""
}
