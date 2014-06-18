// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	NewResourceCatalog = newResourceCatalog
	NewResource        = newResource
	PutResourceTxn     = &putResourceTxn
)

func GetResourceCatalog(ms ManagedStorage) ResourceCatalog {
	return ms.(*managedStorage).resourceCatalog
}

func PutManagedResource(ms ManagedStorage, managedResource ManagedResource, id string) (string, error) {
	return ms.(*managedStorage).putManagedResource(managedResource, id)
}

func ResourceStoragePath(ms ManagedStorage, env_uuid, user, resourcePath string) string {
	return ms.(*managedStorage).resourceStoragePath(env_uuid, user, resourcePath)
}
