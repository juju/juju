// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import "github.com/juju/worker/v2"

var (
	NewManagedFilesystemSource = &newManagedFilesystemSource
)

func StorageWorker(parent worker.Worker, appName string) (worker.Worker, bool) {
	p := parent.(*provisioner)
	return p.getApplicationWorker(appName)
}

func NewStorageWorker(parent worker.Worker, appName string) {
	p := parent.(*provisioner)
	p.saveApplicationWorker(appName, &storageProvisioner{})
}
