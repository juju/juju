// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import "github.com/juju/worker/v2"

func AppWorker(parent worker.Worker, appName string) (*applicationWorker, bool) {
	p := parent.(*provisioner)
	return p.getApplicationWorker(appName)
}

func NewAppWorker(parent worker.Worker, appName string) {
	p := parent.(*provisioner)
	p.saveApplicationWorker(appName, &applicationWorker{})
}
