// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/state"
)

// TODO: Move all generated mocks out of the mocks directory and directly into
// the common or common_test package.

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/authorizer_mock.go github.com/juju/juju/apiserver/common Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCommandService,CloudService,ControllerConfigState,ControllerConfigService,ExternalControllerService,ToolsFinder,ToolsURLGetter,APIHostPortsForAgentsGetter,ToolsStorageGetter,ModelAgentService,MachineRebootService,EnsureDeadMachineService,WatchableMachineService,UnitStateService,MachineService,StatusService,LeadershipPinningBackend,LeadershipMachine,AgentPasswordService,StubService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/storage_mock.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mocks.go github.com/juju/juju/state EntityFinder,Entity
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/status_mock.go github.com/juju/juju/core/status StatusGetter,StatusSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore_mock.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type entityWithError interface {
	state.Entity
	error() error
}

type fakeState struct {
	entities map[names.Tag]entityWithError
}

func (st *fakeState) FindEntity(tag names.Tag) (state.Entity, error) {
	entity, ok := st.entities[tag]
	if !ok {
		return nil, errors.NotFoundf("entity %q", tag)
	}
	if err := entity.error(); err != nil {
		return nil, err
	}
	return entity, nil
}

type fetchError string

func (f fetchError) error() error {
	if f == "" {
		return nil
	}
	return fmt.Errorf("%s", string(f))
}
