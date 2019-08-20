// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
)

//go:generate mockgen -package mocks -destination mocks/upgradesteps_mock.go github.com/juju/juju/apiserver/facades/agent/upgradesteps UpgradeStepsState,Machine
//go:generate mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state EntityFinder,Entity

var logger = loggo.GetLogger("juju.apiserver.upgradesteps")

type UpgradeStepsV1 interface {
	ResetKVMMachineModificationStatusIdle(params.Entity) (params.ErrorResult, error)
}

type UpgradeStepsAPI struct {
	st          UpgradeStepsState
	resources   facade.Resources
	authorizer  facade.Authorizer
	getAuthFunc common.GetAuthFunc
}

// using apiserver/facades/client/cloud as an example.
var (
	_ UpgradeStepsV1 = (*UpgradeStepsAPI)(nil)
)

// NewFacadeV1 is used for API registration.
func NewFacadeV1(ctx facade.Context) (*UpgradeStepsAPI, error) {
	st := &upgradeStepsStateShim{State: ctx.State()}
	return NewUpgradeStepsAPI(st, ctx.Resources(), ctx.Auth())
}

func NewUpgradeStepsAPI(st UpgradeStepsState,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*UpgradeStepsAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthController() {
		return nil, common.ErrPerm
	}

	getAuthFunc := common.AuthFuncForMachineAgent(authorizer)
	return &UpgradeStepsAPI{
		st:          st,
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

// ResetKVMMachineModificationStatusIdle sets the modification status
// of a kvm machine to idle if it is in an error state before upgrade.
// Related to lp:1829393.
func (api *UpgradeStepsAPI) ResetKVMMachineModificationStatusIdle(arg params.Entity) (params.ErrorResult, error) {
	var result params.ErrorResult
	canAccess, err := api.getAuthFunc()
	if err != nil {
		return result, errors.Trace(err)
	}

	mTag, err := names.ParseMachineTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	m, err := api.getMachine(canAccess, mTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	if m.ContainerType() != instance.KVM {
		// noop
		return result, nil
	}

	modStatus, err := m.ModificationStatus()
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}

	if modStatus.Status == status.Error {
		err = m.SetModificationStatus(status.StatusInfo{Status: status.Idle})
		result.Error = common.ServerError(err)
	}

	return result, nil
}

func (api *UpgradeStepsAPI) getMachine(canAccess common.AuthFunc, tag names.MachineTag) (Machine, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	entity, err := api.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a
	// machine.
	var machine Machine
	var ok bool
	if machine, ok = entity.(Machine); !ok {
		return nil, errors.NotValidf("machine entity")
	}
	return machine, nil
}
