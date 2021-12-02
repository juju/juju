// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	gc "gopkg.in/check.v1"
)

var (
	NewManagedFilesystemSource = &newManagedFilesystemSource
)

func StorageWorker(parent worker.Worker, appName string) (worker.Worker, bool) {
	p := parent.(*provisioner)
	return p.getApplicationWorker(appName)
}

func NewStorageWorker(c *gc.C, parent worker.Worker, appName string) {
	p := parent.(*provisioner)
	cfg := p.config
	cfg.Scope = names.NewApplicationTag(appName)
	w, err := NewStorageProvisioner(cfg)
	c.Assert(err, jc.ErrorIsNil)
	p.saveApplicationWorker(appName, w)
	_ = p.catacomb.Add(w)
}
