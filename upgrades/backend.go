// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

// StateBackend provides an interface for upgrading the global state database.
type StateBackend interface {
	AllModels() ([]Model, error)
	ControllerUUID() string

	StripLocalUserDomain() error
	RenameAddModelPermission() error
	AddMigrationAttempt() error
	AddLocalCharmSequences() error
	UpdateLegacyLXDCloudCredentials(string, cloud.Credential) error
	UpgradeNoProxyDefaults() error
	AddNonDetachableStorageMachineId() error
	RemoveNilValueApplicationSettings() error
	AddControllerLogCollectionsSizeSettings() error
	AddStatusHistoryPruneSettings() error
	AddStorageInstanceConstraints() error
	SplitLogCollections() error
	AddUpdateStatusHookSettings() error
	CorrectRelationUnitCounts() error
}

// Model is an interface providing access to the details of a model within the
// controller.
type Model interface {
	Config() (*config.Config, error)
	CloudSpec() (environs.CloudSpec, error)
}

// NewStateBackend returns a new StateBackend using a *state.State object.
func NewStateBackend(st *state.State) StateBackend {
	return stateBackend{st}
}

type stateBackend struct {
	st *state.State
}

func (s stateBackend) AllModels() ([]Model, error) {
	models, err := s.st.AllModels()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]Model, len(models))
	for i, m := range models {
		out[i] = &modelShim{s.st, m}
	}
	return out, nil
}

func (s stateBackend) ControllerUUID() string {
	return s.st.ControllerUUID()
}

func (s stateBackend) StripLocalUserDomain() error {
	return state.StripLocalUserDomain(s.st)
}

func (s stateBackend) RenameAddModelPermission() error {
	return state.RenameAddModelPermission(s.st)
}

func (s stateBackend) AddMigrationAttempt() error {
	return state.AddMigrationAttempt(s.st)
}

func (s stateBackend) AddLocalCharmSequences() error {
	return state.AddLocalCharmSequences(s.st)
}

func (s stateBackend) UpdateLegacyLXDCloudCredentials(endpoint string, credential cloud.Credential) error {
	return state.UpdateLegacyLXDCloudCredentials(s.st, endpoint, credential)
}

func (s stateBackend) UpgradeNoProxyDefaults() error {
	return state.UpgradeNoProxyDefaults(s.st)
}

func (s stateBackend) AddNonDetachableStorageMachineId() error {
	return state.AddNonDetachableStorageMachineId(s.st)
}

func (s stateBackend) RemoveNilValueApplicationSettings() error {
	return state.RemoveNilValueApplicationSettings(s.st)
}

func (s stateBackend) AddControllerLogCollectionsSizeSettings() error {
	return state.AddControllerLogCollectionsSizeSettings(s.st)
}

func (s stateBackend) AddStatusHistoryPruneSettings() error {
	return state.AddStatusHistoryPruneSettings(s.st)
}

func (s stateBackend) AddUpdateStatusHookSettings() error {
	return state.AddUpdateStatusHookSettings(s.st)
}

func (s stateBackend) AddStorageInstanceConstraints() error {
	return state.AddStorageInstanceConstraints(s.st)
}

func (s stateBackend) SplitLogCollections() error {
	return state.SplitLogCollections(s.st)
}

func (s stateBackend) CorrectRelationUnitCounts() error {
	return state.CorrectRelationUnitCounts(s.st)
}

type modelShim struct {
	st *state.State
	m  *state.Model
}

func (m *modelShim) Config() (*config.Config, error) {
	return m.m.Config()
}

func (m *modelShim) CloudSpec() (environs.CloudSpec, error) {
	cloudName := m.m.Cloud()
	regionName := m.m.CloudRegion()
	credentialTag, _ := m.m.CloudCredential()
	return stateenvirons.CloudSpec(m.st, cloudName, regionName, credentialTag)
}
