// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/metrics"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var (
	MergeWindowsEnvironment = mergeWindowsEnvironment
	SearchHook              = searchHook
	HookCommand             = hookCommand
	LookPath                = lookPath
	ValidatePortRange       = validatePortRange
	TryOpenPorts            = tryOpenPorts
	TryClosePorts           = tryClosePorts
)

func RunnerPaths(rnr Runner) Paths {
	return rnr.(*runner).paths
}

type LeadershipContextFunc func(LeadershipSettingsAccessor, leadership.Tracker) LeadershipContext

func PatchNewLeadershipContext(f LeadershipContextFunc) func() {
	var old LeadershipContextFunc
	old, newLeadershipContext = newLeadershipContext, f
	return func() { newLeadershipContext = old }
}

func UpdateCachedSettings(f0 Factory, relId int, unitName string, settings params.Settings) {
	f := f0.(*factory)
	cf := f.contextFactory.(*contextFactory)
	members := cf.relationCaches[relId].members
	if members[unitName] == nil {
		members[unitName] = params.Settings{}
	}
	for key, value := range settings {
		members[unitName][key] = value
	}
}

func CachedSettings(f0 Factory, relId int, unitName string) (params.Settings, bool) {
	f := f0.(*factory)
	cf := f.contextFactory.(*contextFactory)
	settings, found := cf.relationCaches[relId].members[unitName]
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

func ContextEnvInfo(ctx Context) (name, uuid string) {
	hctx := ctx.(*HookContext)
	return hctx.envName, hctx.uuid
}

func ContextMachineTag(ctx Context) names.MachineTag {
	hctx := ctx.(*HookContext)
	return hctx.assignedMachineTag
}

func GetStubActionContext(in map[string]interface{}) *HookContext {
	return &HookContext{
		actionData: &ActionData{
			ResultsMap: in,
		},
	}
}

func PatchCachedStatus(ctx Context, status, info string, data map[string]interface{}) func() {
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
		metricsRecorder:    nil,
		definedMetrics:     charmMetrics,
		actionData:         actionData,
		pendingPorts:       make(map[PortRange]PortRangeInfo),
		assignedMachineTag: assignedMachineTag,
	}
	if canAddMetrics {
		charmURL, err := unit.CharmURL()
		if err != nil {
			return nil, err
		}
		ctx.metricsRecorder, err = metrics.NewJSONMetricRecorder(
			paths.GetMetricsSpoolDir(),
			charmMetrics.Metrics,
			charmURL.String())
		if err != nil {
			return nil, err
		}
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

// NewEnvironmentHookContext exists purely to set the fields used in rs.
// The returned value is not otherwise valid.
func NewEnvironmentHookContext(
	id, envUUID, envName, unitName, meterCode, meterInfo, availZone string,
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
		availabilityzone:   availZone,
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
		relationId: {
			endpointName: endpointName,
			relationId:   relationId,
		},
	}
}

func (ctx *HookContext) StorageAddConstraints() map[string][]params.StorageConstraints {
	return ctx.storageAddConstraints
}

// PatchMetricsRecorder patches the metrics writer used by the context with a new
// object.
func PatchMetricsRecorder(ctx jujuc.Context, writer MetricsRecorder) func() {
	hctx := ctx.(*HookContext)
	oldRecorder := hctx.metricsRecorder
	hctx.metricsRecorder = writer
	return func() {
		hctx.metricsRecorder = oldRecorder
	}
}
