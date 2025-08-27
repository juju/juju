// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/instancemutater_mock.go github.com/juju/juju/apiserver/facades/agent/instancemutater InstanceMutatorWatcher,InstanceMutaterState,Machine,Unit,Application,Charm
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity,Lifer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/watcher_mock.go github.com/juju/juju/state NotifyWatcher,StringsWatcher

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// NewTestAPI is exported for use by tests that need
// to create an instance-mutater API facade.
func NewTestAPI(
	st InstanceMutaterState,
	mutatorWatcher InstanceMutatorWatcher,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*InstanceMutaterAPIV3, error) {
	api, err := NewTestAPIV4(st, mutatorWatcher, resources, authorizer)
	if err != nil {
		return nil, err
	}

	return &InstanceMutaterAPIV3{api}, nil
}

// NewTestAPIV4 is exported for use by tests that need
// to create an instance-mutater API V4 facade.
func NewTestAPIV4(
	st InstanceMutaterState,
	mutatorWatcher InstanceMutatorWatcher,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*InstanceMutaterAPIV4, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &InstanceMutaterAPIV4{LifeGetter: common.NewLifeGetter(st, getAuthFunc),
		st:          st,
		watcher:     mutatorWatcher,
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

// NewTestLxdProfileWatcher is used by the lxd profile tests.
func NewTestLxdProfileWatcher(c *gc.C, machine Machine, backend InstanceMutaterState) *machineLXDProfileWatcher {
	w, err := newMachineLXDProfileWatcher(MachineLXDProfileWatcherConfig{
		backend: backend,
		machine: machine,
	})
	c.Assert(err, jc.ErrorIsNil)
	return w
}

// NewEmptyCharmShim returns a charm shim that satisfies the Charm indirection.
// CAUTION. This is only suitable for testing scenarios where members of the
// inner charm document have zero values.
// Calls relying on the inner state reference will cause a nil-reference panic.
func NewEmptyCharmShim() *charmShim {
	return &charmShim{
		Charm: &state.Charm{},
	}
}
