// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"github.com/juju/utils/set"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/discoverspaces"
	"github.com/juju/juju/worker/gate"
)

type fakeWorker struct {
	worker.Worker
}

type fakeAPICaller struct {
	base.APICaller
}

type fakeFacade struct {
	discoverspaces.Facade
}

type fakeEnviron struct {
	environs.NetworkingEnviron
}

func fakeNewName(_ string, _ set.Strings) string {
	panic("fake")
}

type fakeUnlocker struct {
	gate.Unlocker
}
