// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	NewResourceCatalog = newResourceCatalog
	NewResource        = newResource
	PutResourceTxn     = &putResourceTxn
	RequestExpiry      = &requestExpiry
	AfterFunc          = &afterFunc
)

func GetResourceCatalog(ms ManagedStorage) ResourceCatalog {
	return ms.(*managedStorage).resourceCatalog
}

func PutManagedResource(ms ManagedStorage, managedResource ManagedResource, id string) (string, error) {
	return ms.(*managedStorage).putManagedResource(managedResource, id)
}

func ResourceStoragePath(ms ManagedStorage, envUUID, user, resourcePath string) (string, error) {
	return ms.(*managedStorage).resourceStoragePath(envUUID, user, resourcePath)
}

func RequestQueueLength(ms ManagedStorage) int {
	return len(ms.(*managedStorage).queuedRequests)
}
