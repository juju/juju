// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker/caasoperator/commands"
)

type hookAPI interface {
	ApplicationConfig() (charm.Settings, error)
	NetworkInfo([]string, *int) (map[string]params.NetworkInfoResult, error)
	ApplicationStatus() (params.ApplicationStatusResult, error)
	SetApplicationStatus(status.Status, string, map[string]interface{}) error
	SetContainerSpec(string, string) error
}

type contextFactoryAPI interface {
	APIAddresses() ([]string, error)
	ProxySettings() (proxy.Settings, error)
}

type relationUnitAPI interface {
	Id() int
	Endpoint() string
	Suspended() bool
	SetStatus(status relation.Status) error
	LocalSettings() (commands.Settings, error)
	RemoteSettings(string) (commands.Settings, error)
	WriteSettings(commands.Settings) error
}
