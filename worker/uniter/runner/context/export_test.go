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
	"github.com/juju/juju/worker/uniter/runner/context/mocks"
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

func NewMockUnitHookContext(mockUnit *mocks.MockHookUnit) *HookContext {
	return &HookContext{
		unit: mockUnit,
	}
}

func NewMockUnitHookContextWithState(mockUnit *mocks.MockHookUnit, state *uniter.State) *HookContext {
	return &HookContext{
		unit:  mockUnit,
		state: state,
	}
}

// SetEnvironmentHookContextRelation exists purely to set the fields used in hookVars.
// It makes no assumptions about the validity of context.
func SetEnvironmentHookContextRelation(context *HookContext, relationId int, endpointName, remoteUnitName, remoteAppName, departingUnitName string) {
	context.relationId = relationId
	context.remoteUnitName = remoteUnitName
	context.remoteApplicationName = remoteAppName
	context.relations = map[int]*ContextRelation{
		relationId: {
			endpointName: endpointName,
			relationId:   relationId,
		},
	}
	context.departingUnitName = departingUnitName
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

func WithActionContext(ctx *HookContext, in map[string]interface{}, cancel <-chan struct{}) {
	ctx.actionData = &ActionData{
		Tag:        names.NewActionTag("2"),
		ResultsMap: in,
		Cancel:     cancel,
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

// ModelHookContextParams encapsulates the parameters for a NewModelHookContext call.
type ModelHookContextParams struct {
	ID        string
	HookName  string
	ModelUUID string
	ModelName string
	UnitName  string

	MeterCode string
	MeterInfo string
	SLALevel  string

	AvailZone    string
	APIAddresses []string

	LegacyProxySettings proxy.Settings
	JujuProxySettings   proxy.Settings

	MachineTag names.MachineTag
}

// NewModelHookContext exists purely to set the fields used in rs.
// The returned value is not otherwise valid.
func NewModelHookContext(p ModelHookContextParams) *HookContext {
	return &HookContext{
		id:                  p.ID,
		hookName:            p.HookName,
		unitName:            p.UnitName,
		uuid:                p.ModelUUID,
		modelName:           p.ModelName,
		apiAddrs:            p.APIAddresses,
		legacyProxySettings: p.LegacyProxySettings,
		jujuProxySettings:   p.JujuProxySettings,
		meterStatus: &meterStatus{
			code: p.MeterCode,
			info: p.MeterInfo,
		},
		relationId:         -1,
		assignedMachineTag: p.MachineTag,
		availabilityzone:   p.AvailZone,
		slaLevel:           p.SLALevel,
		principal:          p.UnitName,
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

func UpdateCachedAppSettings(cf0 ContextFactory, relId int, appName string, settings params.Settings) {
	cf := cf0.(*contextFactory)
	applications := cf.relationCaches[relId].applications
	if applications[appName] == nil {
		applications[appName] = params.Settings{}
	}
	for key, value := range settings {
		applications[appName][key] = value
	}
}

func CachedSettings(cf0 ContextFactory, relId int, unitName string) (params.Settings, bool) {
	cf := cf0.(*contextFactory)
	settings, found := cf.relationCaches[relId].members[unitName]
	return settings, found
}

func CachedAppSettings(cf0 ContextFactory, relId int, appName string) (params.Settings, bool) {
	cf := cf0.(*contextFactory)
	settings, found := cf.relationCaches[relId].applications[appName]
	return settings, found
}

func (ctx *HookContext) SLALevel() string {
	return ctx.slaLevel
}
