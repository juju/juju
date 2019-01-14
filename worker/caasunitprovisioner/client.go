// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	apicaasunitprovisioner "github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// Client provides an interface for interacting with the
// CAASUnitProvisioner API. Subsets of this should be passed
// to the CAASUnitProvisioner worker.
type Client interface {
	ApplicationGetter
	ApplicationUpdater
	ProvisioningInfoGetter
	LifeGetter
	UnitUpdater
	ProvisioningStatusSetter
}

// ApplicationGetter provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications in the
// model, and fetching their details.
type ApplicationGetter interface {
	WatchApplications() (watcher.StringsWatcher, error)
	ApplicationConfig(string) (application.ConfigAttributes, error)
	WatchApplicationScale(string) (watcher.NotifyWatcher, error)
	ApplicationScale(string) (int, error)
}

// ApplicationUpdater provides an interface for updating
// Juju applications from changes in the cloud.
type ApplicationUpdater interface {
	UpdateApplicationService(arg params.UpdateApplicationServiceArg) error
}

// ProvisioningInfoGetter provides an interface for
// watching and getting the pod spec and other info
// needed to provision an application.
type ProvisioningInfoGetter interface {
	ProvisioningInfo(appName string) (*apicaasunitprovisioner.ProvisioningInfo, error)
	WatchPodSpec(appName string) (watcher.NotifyWatcher, error)
}

// LifeGetter provides an interface for getting the
// lifecycle state value for an application or unit.
type LifeGetter interface {
	Life(string) (life.Value, error)
}

// UnitUpdater provides an interface for updating
// Juju units from changes in the cloud.
type UnitUpdater interface {
	UpdateUnits(arg params.UpdateApplicationUnits) error
}

// ProvisioningStatusSetter provides an interface for
// setting status information.
type ProvisioningStatusSetter interface {
	// SetOperatorStatus sets the status for the application operator.
	SetOperatorStatus(appName string, status status.Status, message string, data map[string]interface{}) error
}
