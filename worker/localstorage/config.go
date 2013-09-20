// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localstorage

// LocalStorageConfig is an interface that, if implemented, may be used
// to configure a machine agent for use with the localstorage worker in
// this package.
type LocalStorageConfig interface {
	StorageDir() string
	StorageAddr() string
	SharedStorageDir() string
	SharedStorageAddr() string
}
