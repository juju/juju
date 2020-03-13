// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

// NewTestAPI is exported for use by tests that need
// to create an instance-mutater API facade.
func NewTestAPI(
	st InstanceMutaterState,
	model ModelCache,
	resources facade.Resources,
	authorizer facade.Authorizer,
	machineFunc EntityMachineFunc,
) (*InstanceMutaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &InstanceMutaterAPI{
		LifeGetter:  common.NewLifeGetter(st, getAuthFunc),
		st:          st,
		model:       model,
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
		machineFunc: machineFunc,
	}, nil
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
