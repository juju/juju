// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	jujutesting "github.com/juju/testing"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/worker/caasapplicationprovisioner"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/worker/caasapplicationprovisioner CAASBroker
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/facade_mock.go github.com/juju/juju/worker/caasapplicationprovisioner CAASProvisionerFacade
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/broker_application_mock.go github.com/juju/juju/caas Application

type mockFacade struct {
	caasapplicationprovisioner.CAASProvisionerFacade
	jujutesting.Stub
	appWatcher *watchertest.MockStringsWatcher
}

func (f *mockFacade) WatchApplications() (watcher.StringsWatcher, error) {
	f.MethodCall(f, "WatchApplications")
	return f.appWatcher, f.NextErr()
}

type mockNotifyWorker struct {
	worker.Worker
	jujutesting.Stub
}

func (w *mockNotifyWorker) Notify() {
	w.MethodCall(w, "Notify")
}
