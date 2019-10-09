// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"
	"github.com/juju/proxy"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

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

type HookContextParams struct {
	Unit                *uniter.Unit
	State               *uniter.State
	ID                  string
	UUID                string
	ModelName           string
	RelationID          int
	RemoteUnitName      string
	Relations           map[int]*ContextRelation
	APIAddrs            []string
	LegacyProxySettings proxy.Settings
	JujuProxySettings   proxy.Settings
	CanAddMetrics       bool
	CharmMetrics        *charm.Metrics
	ActionData          *ActionData
	AssignedMachineTag  names.MachineTag
	Paths               Paths
	Clock               Clock
}

func NewHookContext(hcParams HookContextParams) (*HookContext, error) {
	ctx := &HookContext{
		unit:                hcParams.Unit,
		state:               hcParams.State,
		id:                  hcParams.ID,
		uuid:                hcParams.UUID,
		modelName:           hcParams.ModelName,
		unitName:            hcParams.Unit.Name(),
		relationId:          hcParams.RelationID,
		remoteUnitName:      hcParams.RemoteUnitName,
		relations:           hcParams.Relations,
		apiAddrs:            hcParams.APIAddrs,
		legacyProxySettings: hcParams.LegacyProxySettings,
		jujuProxySettings:   hcParams.JujuProxySettings,
		actionData:          hcParams.ActionData,
		pendingPorts:        make(map[PortRange]PortRangeInfo),
		assignedMachineTag:  hcParams.AssignedMachineTag,
		clock:               hcParams.Clock,
	}
	// Get and cache the addresses.
	var err error
	ctx.publicAddress, err = hcParams.Unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	ctx.privateAddress, err = hcParams.Unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return nil, err
	}
	ctx.availabilityzone, err = hcParams.Unit.AvailabilityZone()
	if err != nil {
		return nil, err
	}
	ctx.machinePorts, err = hcParams.State.AllMachinePorts(ctx.assignedMachineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	statusCode, statusInfo, err := hcParams.Unit.MeterStatus()
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

func WithActionContext(ctx *HookContext, in map[string]interface{}) {
	ctx.actionData = &ActionData{
		Tag:        names.NewActionTag("u-1"),
		ResultsMap: in,
	}
}

type LeadershipContextFunc func(LeadershipSettingsAccessor, leadership.Tracker, string) LeadershipContext

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
	id, modelUUID, modelName, unitName, meterCode, meterInfo, slaLevel, availZone string,
	apiAddresses []string, legacyProxySettings proxy.Settings, jujuProxySettings proxy.Settings,
	machineTag names.MachineTag,
) *HookContext {
	return &HookContext{
		id:                  id,
		unitName:            unitName,
		uuid:                modelUUID,
		modelName:           modelName,
		apiAddrs:            apiAddresses,
		legacyProxySettings: legacyProxySettings,
		jujuProxySettings:   jujuProxySettings,
		meterStatus: &meterStatus{
			code: meterCode,
			info: meterInfo,
		},
		relationId:         -1,
		assignedMachineTag: machineTag,
		availabilityzone:   availZone,
		slaLevel:           slaLevel,
		principal:          unitName,
		cloudAPIVersion:    "6.66",
	}
}

func ContextEnvInfo(hctx *HookContext) (name, uuid string) {
	return hctx.modelName, hctx.uuid
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

func (ctx *HookContext) SLALevel() string {
	return ctx.slaLevel
}
