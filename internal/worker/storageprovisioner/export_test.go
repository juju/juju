// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
)

var (
	NewManagedFilesystemSource     = &newManagedFilesystemSource
	DefaultDependentChangesTimeout = &defaultDependentChangesTimeout
)

func StorageWorker(parent worker.Worker, appName string) (worker.Worker, bool) {
	p := parent.(*provisioner)
	return p.getApplicationWorker(appName)
}

func NewStorageWorker(c *tc.C, parent worker.Worker, appName string) {
	p := parent.(*provisioner)
	cfg := p.config
	cfg.Scope = names.NewApplicationTag(appName)
	w, err := NewStorageProvisioner(cfg)
	c.Assert(err, tc.ErrorIsNil)
	p.saveApplicationWorker(appName, w)
	_ = p.catacomb.Add(w)
}
