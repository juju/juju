// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

// ExportVersions lists each semantic version for which there is a new
// export format.
// To generate new export types and logic, add the current semantic version
// in string form, then run `go generate` from the generate/export directory.
// If the version currently being worked on has not been released,
// the generation can be run repeatedly for the same version.
var ExportVersions = []string{
	"4.0.1",
	"4.0.2",
}
