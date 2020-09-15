// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewallerembedded

import (
	"github.com/juju/charm/v8"

	charmscommon "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/worker/caasfirewallerembedded Client,CAASFirewallerAPI,LifeGetter
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/core/watcher StringsWatcher,NotifyWatcher
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/worker_mock.go github.com/juju/worker/v2 Worker
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/api_base_mock.go github.com/juju/juju/api/base APICaller

// Client provides an interface for interacting with the
// CAASFirewallerAPI. Subsets of this should be passed
// to the CAASFirewaller worker.
type Client interface {
	CAASFirewallerAPI
	LifeGetter
}

// CAASFirewallerAPI provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications in the
// model, and fetching their details.
type CAASFirewallerAPI interface {
	WatchApplications() (watcher.StringsWatcher, error)
	WatchApplication(string) (watcher.NotifyWatcher, error)
	WatchOpenedPorts() (watcher.StringsWatcher, error)

	IsExposed(string) (bool, error)
	ApplicationConfig(string) (application.ConfigAttributes, error)

	CharmInfo(string) (*charmscommon.CharmInfo, error)
	ApplicationCharmURL(string) (*charm.URL, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application.
type LifeGetter interface {
	Life(string) (life.Value, error)
}
