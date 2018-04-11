// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package version contains versioning information about packages that juju supports.
package version

// SupportedLts returns the latest LTS that Juju supports and is compatible with.
// For example, Juju 2.3.x series cannot be run on "bionic"
// as mongo version that it depends on (3.2 and less) is not packaged for bionic.
func SupportedLTS() string {
	return "xenial"
}
