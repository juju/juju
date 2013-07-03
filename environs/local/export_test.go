// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

var Provider = provider

func SetDefaultRootDir(rootdir string) (old string) {
	old, defaultRootDir = defaultRootDir, rootdir
	return
}
