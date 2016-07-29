// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

// Version describes the current version of the code being run.
type Version struct {
	GitCommit string
	Version   string
}

// VersionInfo is a variable representing the version of the currently
// executing code. Builds of the system where the version information
// is required must arrange to provide the correct values for this
// variable. One possible way to do this is to create an init() function
// that updates this variable, please see init.go.tmpl to see an example.
var VersionInfo = unknownVersion

var unknownVersion = Version{
	GitCommit: "unknown git commit",
	Version:   "unknown version",
}
