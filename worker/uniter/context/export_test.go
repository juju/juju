// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v4"

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

func UpdateCachedSettings(f0 Factory, relId int, unitName string, settings params.RelationSettings) {
	f := f0.(*factory)
	members := f.relationCaches[relId].members
	if members[unitName] == nil {
		members[unitName] = params.RelationSettings{}
	}
	for key, value := range settings {
		members[unitName][key] = value
	}
}

func CachedSettings(f0 Factory, relId int, unitName string) (params.RelationSettings, bool) {
	f := f0.(*factory)
	settings, found := f.relationCaches[relId].members[unitName]
	return settings, found
}

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

func (c *HookContext) EnvInfo() (name, uuid string) {
	return c.envName, c.uuid
}

func (c *HookContext) AssignedMachineTag() names.MachineTag {
	return c.assignedMachineTag
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
	metrics *charm.Metrics,
	actionData *ActionData,
	assignedMachineTag names.MachineTag,
) (*HookContext, error) {
	ctx := &HookContext{
		unit:               unit,
		state:              state,
		id:                 id,
		uuid:               uuid,
		envName:            envName,
		unitName:           unit.Name(),
		relationId:         relationId,
		remoteUnitName:     remoteUnitName,
		relations:          relations,
		apiAddrs:           apiAddrs,
		serviceOwner:       serviceOwner,
		proxySettings:      proxySettings,
		canAddMetrics:      canAddMetrics,
		definedMetrics:     metrics,
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

// NewEnvironmentHookContext exists purely to set the fields used in hookVars.
// The returned value is not otherwise valid.
func NewEnvironmentHookContext(
	id, envUUID, envName, unitName, meterCode, meterInfo string,
	apiAddresses []string, proxySettings proxy.Settings,
	machineTag names.MachineTag,
) *HookContext {
	return &HookContext{
		id:            id,
		unitName:      unitName,
		uuid:          envUUID,
		envName:       envName,
		apiAddrs:      apiAddresses,
		proxySettings: proxySettings,
		meterStatus: &meterStatus{
			code: meterCode,
			info: meterInfo,
		},
		relationId:         -1,
		assignedMachineTag: machineTag,
	}
}

// SetEnvironmentHookContextRelation exists purely to set the fields used in hookVars.
// It makes no assumptions about the validity of context.
func SetEnvironmentHookContextRelation(
	context *HookContext,
	relationId int, endpointName, remoteUnitName string,
) {
	context.relationId = relationId
	context.remoteUnitName = remoteUnitName
	context.relations = map[int]*ContextRelation{
		relationId: &ContextRelation{
			endpointName: endpointName,
			relationId:   relationId,
		},
	}
}
