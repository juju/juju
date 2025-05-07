// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/instancemutater_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater InstanceMutatorWatcher,InstanceMutaterState,Machine,Unit,Application,MachineService,ApplicationService,ModelInfoService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity,Lifer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/state NotifyWatcher,StringsWatcher

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

// NewTestAPI is exported for use by tests that need
// to create an instance-mutater API facade.
func NewTestAPI(
	c *tc.C,
	st InstanceMutaterState,
	machineService MachineService,
	applicationService ApplicationService,
	modelInfoService ModelInfoService,
	mutatorWatcher InstanceMutatorWatcher,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)

	return &InstanceMutaterAPI{
		LifeGetter:         common.NewLifeGetter(st, getAuthFunc),
		st:                 st,
		machineService:     machineService,
		applicationService: applicationService,
		modelInfoService:   modelInfoService,
		watcher:            mutatorWatcher,
		resources:          resources,
		authorizer:         authorizer,
		getAuthFunc:        getAuthFunc,
		logger:             loggertesting.WrapCheckLog(c),
	}, nil
}

// NewTestLxdProfileWatcher is used by the lxd profile tests.
func NewTestLxdProfileWatcher(c *tc.C, machine Machine, backend InstanceMutaterState, machineService MachineService, applicationService ApplicationService) *machineLXDProfileWatcher {
	w, err := newMachineLXDProfileWatcher(
		context.Background(),
		MachineLXDProfileWatcherConfig{
			backend:            backend,
			machine:            machine,
			logger:             loggertesting.WrapCheckLog(c),
			machineService:     machineService,
			applicationService: applicationService,
		})
	c.Assert(err, jc.ErrorIsNil)
	return w
}
