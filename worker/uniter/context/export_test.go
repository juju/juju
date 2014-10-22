// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
)

var (
	MergeEnvironment  = mergeEnvironment
	SearchHook        = searchHook
	HookCommand       = hookCommand
	LookPath          = lookPath
	ValidatePortRange = validatePortRange
	TryOpenPorts      = tryOpenPorts
	TryClosePorts     = tryClosePorts
)

// PatchMeterStatus changes the meter status of the context.
func (ctx *HookContext) PatchMeterStatus(code, info string) func() {
	oldMeterStatus := ctx.meterStatus
	ctx.meterStatus = &meterStatus{
		code: code,
		info: info,
	}
	return func() {
		ctx.meterStatus = oldMeterStatus
	}
}

func (c *HookContext) ActionResultsMap() map[string]interface{} {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ResultsMap
}

func (c *HookContext) ActionFailed() bool {
	if c.actionData == nil {
		panic("context not running an action")
	}
	return c.actionData.ActionFailed
}

func (c *HookContext) EnvInfo() (name, uuid string) {
	return c.envName, c.uuid
}

func (c *HookContext) ActionData() *ActionData {
	return c.actionData
}

func GetStubActionContext(in map[string]interface{}) *HookContext {
	return &HookContext{
		actionData: &ActionData{
			ResultsMap: in,
		},
	}
}

func NewHookContext(
	unit *uniter.Unit,
	state *uniter.State,
	id,
	uuid,
	envName string,
	relationId int,
	remoteUnitName string,
	relations map[int]*ContextRelation,
	apiAddrs []string,
	serviceOwner names.UserTag,
	proxySettings proxy.Settings,
	canAddMetrics bool,
	actionData *ActionData,
	assignedMachineTag names.MachineTag,
) (*HookContext, error) {
	ctx := &HookContext{
		unit:               unit,
		state:              state,
		id:                 id,
		uuid:               uuid,
		envName:            envName,
		relationId:         relationId,
		remoteUnitName:     remoteUnitName,
		relations:          relations,
		apiAddrs:           apiAddrs,
		serviceOwner:       serviceOwner,
		proxySettings:      proxySettings,
		canAddMetrics:      canAddMetrics,
		actionData:         actionData,
		pendingPorts:       make(map[PortRange]PortRangeInfo),
		assignedMachineTag: assignedMachineTag,
	}
	// Get and cache the addresses.
	var err error
	ctx.publicAddress, err = unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	ctx.privateAddress, err = unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	ctx.machinePorts, err = state.AllMachinePorts(ctx.assignedMachineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	statusCode, statusInfo, err := unit.MeterStatus()
	if err != nil {
		return nil, errors.Annotate(err, "could not retrieve meter status for unit")
	}
	ctx.meterStatus = &meterStatus{
		code: statusCode,
		info: statusInfo,
	}

	return ctx, nil
}
