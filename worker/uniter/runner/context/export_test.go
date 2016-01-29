// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var (
	ValidatePortRange = validatePortRange
	TryOpenPorts      = tryOpenPorts
	TryClosePorts     = tryClosePorts
)

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
	proxySettings proxy.Settings,
	canAddMetrics bool,
	charmMetrics *charm.Metrics,
	actionData *ActionData,
	assignedMachineTag names.MachineTag,
	paths Paths,
	clock clock.Clock,
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
		proxySettings:      proxySettings,
		actionData:         actionData,
		pendingPorts:       make(map[PortRange]PortRangeInfo),
		assignedMachineTag: assignedMachineTag,
		clock:              clock,
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
	ctx.availabilityzone, err = unit.AvailabilityZone()
	if err != nil {
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

// SetEnvironmentHookContextRelation exists purely to set the fields used in hookVars.
// It makes no assumptions about the validity of context.
func SetEnvironmentHookContextRelation(
	context *HookContext,
	relationId int, endpointName, remoteUnitName string,
) {
	context.relationId = relationId
	context.remoteUnitName = remoteUnitName
	context.relations = map[int]*ContextRelation{
		relationId: {
			endpointName: endpointName,
			relationId:   relationId,
		},
	}
}

func PatchCachedStatus(ctx jujuc.Context, status, info string, data map[string]interface{}) func() {
	hctx := ctx.(*HookContext)
	oldStatus := hctx.status
	hctx.status = &jujuc.StatusInfo{
		Status: status,
		Info:   info,
		Data:   data,
	}
	return func() {
		hctx.status = oldStatus
	}
}

func GetStubActionContext(in map[string]interface{}) *HookContext {
	return &HookContext{
		actionData: &ActionData{
			ResultsMap: in,
		},
	}
}

type LeadershipContextFunc func(LeadershipSettingsAccessor, leadership.Tracker) LeadershipContext

func PatchNewLeadershipContext(f LeadershipContextFunc) func() {
	var old LeadershipContextFunc
	old, newLeadershipContext = newLeadershipContext, f
	return func() { newLeadershipContext = old }
}

func StorageAddConstraints(ctx *HookContext) map[string][]params.StorageConstraints {
	return ctx.storageAddConstraints
}

// NewModelHookContext exists purely to set the fields used in rs.
// The returned value is not otherwise valid.
func NewModelHookContext(
	id, modelUUID, envName, unitName, meterCode, meterInfo, availZone string,
	apiAddresses []string, proxySettings proxy.Settings,
	machineTag names.MachineTag,
) *HookContext {
	return &HookContext{
		id:            id,
		unitName:      unitName,
		uuid:          modelUUID,
		envName:       envName,
		apiAddrs:      apiAddresses,
		proxySettings: proxySettings,
		meterStatus: &meterStatus{
			code: meterCode,
			info: meterInfo,
		},
		relationId:         -1,
		assignedMachineTag: machineTag,
		availabilityzone:   availZone,
	}
}

func ContextEnvInfo(hctx *HookContext) (name, uuid string) {
	return hctx.envName, hctx.uuid
}

func ContextMachineTag(hctx *HookContext) names.MachineTag {
	return hctx.assignedMachineTag
}

func UpdateCachedSettings(cf0 ContextFactory, relId int, unitName string, settings params.Settings) {
	cf := cf0.(*contextFactory)
	members := cf.relationCaches[relId].members
	if members[unitName] == nil {
		members[unitName] = params.Settings{}
	}
	for key, value := range settings {
		members[unitName][key] = value
	}
}

func CachedSettings(cf0 ContextFactory, relId int, unitName string) (params.Settings, bool) {
	cf := cf0.(*contextFactory)
	settings, found := cf.relationCaches[relId].members[unitName]
	return settings, found
}
