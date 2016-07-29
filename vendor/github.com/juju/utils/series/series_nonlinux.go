// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package series

// TODO(ericsnow) Refactor dependents so we can remove this for non-linux.

// ReleaseVersion is a function that has no meaning except on linux.
func ReleaseVersion() string {
	return ""
}

func updateLocalSeriesVersions() error {
	return nil
}
