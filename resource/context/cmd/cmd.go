// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

// HookContext exposes the functionality needed by the "resource-*"
// hook commands.
type HookContext interface {
	// Download downloads the named resource and returns
	// the path to which it was downloaded.
	Download(name string) (filePath string, _ error)
}
