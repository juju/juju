// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

var Provider = provider

func SetDefaultStorageDirs(public, private string) (oldPublic, oldPrivate string) {
	oldPublic, defaultPublicStorageDir = defaultPublicStorageDir, public
	oldPrivate, defaultPrivateStorageDir = defaultPrivateStorageDir, private
	return
}

func (c *environConfig) PublicStorageDir() string {
	return c.publicStorageDir()
}

func (c *environConfig) PrivateStorageDir() string {
	return c.privateStorageDir()
}
