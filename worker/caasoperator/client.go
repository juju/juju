// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/status"
	"github.com/juju/juju/watcher"
)

// Client provides an interface for interacting
// with the CAASOperator API. Subsets of this
// should be passed to the CAASOperator worker.
type Client interface {
	CharmGetter
	UnitGetter
	LifeGetter
	ContainerSpecSetter
	StatusSetter
	APIAddressGetter
	ProxySettingsGetter
	ModelName() (string, error)
}

// CharmGetter provides an interface for getting
// the URL and SHA256 hash of the charm currently
// assigned to the application.
type CharmGetter interface {
	Charm(application string) (_ *charm.URL, sha256 string, _ error)
}

// UnitGetter provides an interface for watching for
// the lifecycle state changes (including addition)
// of a specified application's units, and fetching
// their details.
type UnitGetter interface {
	WatchUnits(string) (watcher.StringsWatcher, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application or unit.
type LifeGetter interface {
	Life(string) (life.Value, error)
}

// ContainerSpecSetter provides an interface for
// setting the container spec for the application
// or unit thereof.
type ContainerSpecSetter interface {
	SetContainerSpec(entityName, spec string) error
}

// StatusSetter provides an interface for setting
// the status of a CAAS application.
type StatusSetter interface {
	// SetStatus sets the status of an application.
	SetStatus(
		application string,
		status status.Status,
		info string,
		data map[string]interface{},
	) error
}

// APIAddressGetter provides an interface for getting
// the API addresses of the controller(s).
type APIAddressGetter interface {
	APIAddresses() ([]string, error)
}

// ProxySettingsGetter provides an interface for getting
// the proxy settings of the model.
type ProxySettingsGetter interface {
	ProxySettings() (proxy.Settings, error)
}

// CharmConfigGetter provides an interface for
// watching and getting the application's charm config settings.
type CharmConfigGetter interface {
	CharmConfig(string) (charm.Settings, error)
	WatchCharmConfig(string) (watcher.NotifyWatcher, error)
}

// TODO(caas) - split this up
type contextFactoryAPIAdaptor struct {
	APIAddressGetter
	ProxySettingsGetter
}

type hookAPIAdaptor struct {
	StatusSetter
	CharmConfigGetter
	ContainerSpecSetter

	appName string

	dummyHookAPI
}

func (h *hookAPIAdaptor) CharmConfig() (charm.Settings, error) {
	return h.CharmConfigGetter.CharmConfig(h.appName)
}

func (h *hookAPIAdaptor) SetApplicationStatus(status status.Status, info string, data map[string]interface{}) error {
	return h.StatusSetter.SetStatus(h.appName, status, info, data)
}

// dummyHookAPI is an API placeholder
type dummyHookAPI struct{}

func (h *dummyHookAPI) ApplicationStatus() (params.ApplicationStatusResult, error) {
	return params.ApplicationStatusResult{Application: params.StatusResult{Status: "unknown"}}, nil
}

func (h *dummyHookAPI) NetworkInfo(bindings []string, relId *int) (map[string]params.NetworkInfoResult, error) {
	return make(map[string]params.NetworkInfoResult), nil
}
