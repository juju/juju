// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sync

// SetDefaultToolsLocation allows to swap the default
// tools location for testing.
func SetDefaultToolsLocation(url string) string {
	current := defaultToolsLocation
	defaultToolsLocation = url
	return current
}
