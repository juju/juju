// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package password

// EnsureJujudPassword on linux is a stub. It only does something relevant on
// windows.
var EnsureJujudPassword = func() error {
	return nil
}
