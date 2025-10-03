// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/caas"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
	"github.com/juju/juju/state"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	AddVirtualHostKeys() error
	SplitMigrationStatusMessages() error
	PopulateApplicationStorageUniqueID() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environscloudspec.CloudSpec, error)
}

type CAASGetter interface {
	getAnnotationFunc(appName string, mode string, includeClusterIP bool) (map[string]interface{}, error)
	labelVersionFunc() constants.LabelVersion
}

type getAnnotationFunc func(appName string, mode string, includeClusterIP bool) (map[string]interface{}, error)
type labelVersionFunc func() constants.LabelVersion

// NewStateBackend returns a new StateBackend using a *state.StatePool object.
func NewStateBackend(pool *state.StatePool, broker caas.Broker) StateBackend {
	var getAnnotation getAnnotationFunc = func(appName string, mode string, includeClusterIP bool) (map[string]interface{}, error) {
		service, err := broker.GetService(appName, caas.DeploymentMode(mode), includeClusterIP)
		if err != nil {
			return map[string]interface{}{}, err
		}
		return service.Status.Data, nil
	}
	var labelVersion labelVersionFunc = broker.LabelVersion

	return stateBackend{pool: pool, getAnnotation: getAnnotation, labelVersion: labelVersion}
}

type stateBackend struct {
	pool          *state.StatePool
	getAnnotation getAnnotationFunc
	labelVersion  labelVersionFunc
}

// AddVirtualHostKeys runs an upgrade to
// create missing virtual host keys.
func (s stateBackend) AddVirtualHostKeys() error {
	return state.AddVirtualHostKeys(s.pool)
}

// SplitMigrationStatusMessages runs an upgrade to
// split migration status messages.
func (s stateBackend) SplitMigrationStatusMessages() error {
	return state.SplitMigrationStatusMessages(s.pool)
}

func (s stateBackend) PopulateApplicationStorageUniqueID() error {
	return state.PopulateApplicationStorageUniqueID(s.pool, s.getAnnotation, s.labelVersion)
}
