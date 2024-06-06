// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/proxy"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	jujusecrets "github.com/juju/juju/secrets"
	"github.com/juju/juju/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type HookContextParams struct {
	Unit                *uniter.Unit
	State               State
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
	StorageTag          names.StorageTag
	SecretsClient       SecretsAccessor
	SecretsStore        jujusecrets.BackendsClient
	SecretMetadata      map[string]jujuc.SecretMetadata
	Paths               Paths
	Clock               Clock
}

type stubLeadershipContext struct {
	LeadershipContext
	isLeader bool
}

func (stub *stubLeadershipContext) IsLeader() (bool, error) {
	return stub.isLeader, nil
}

func NewHookContext(hcParams HookContextParams) (*HookContext, error) {
	ctx := &HookContext{
		unit:                   hcParams.Unit,
		state:                  hcParams.State,
		id:                     hcParams.ID,
		uuid:                   hcParams.UUID,
		modelName:              hcParams.ModelName,
		unitName:               hcParams.Unit.Name(),
		relationId:             hcParams.RelationID,
		remoteUnitName:         hcParams.RemoteUnitName,
		relations:              hcParams.Relations,
		apiAddrs:               hcParams.APIAddrs,
		modelType:              model.IAAS,
		legacyProxySettings:    hcParams.LegacyProxySettings,
		jujuProxySettings:      hcParams.JujuProxySettings,
		actionData:             hcParams.ActionData,
		assignedMachineTag:     hcParams.AssignedMachineTag,
		storageTag:             hcParams.StorageTag,
		secretsClient:          hcParams.SecretsClient,
		secretsBackendGetter:   func() (jujusecrets.BackendsClient, error) { return hcParams.SecretsStore, nil },
		secretMetadata:         hcParams.SecretMetadata,
		clock:                  hcParams.Clock,
		logger:                 loggo.GetLogger("test"),
		LeadershipContext:      &stubLeadershipContext{isLeader: true},
		storageAttachmentCache: make(map[names.StorageTag]jujuc.ContextStorageAttachment),
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
	ctx.availabilityZone, err = hcParams.Unit.AvailabilityZone()
	if err != nil {
		return nil, err
	}
	machPorts, err := hcParams.State.OpenedMachinePortRangesByEndpoint(ctx.assignedMachineTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	appPortRanges, err := hcParams.State.OpenedPortRangesByEndpoint()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx.portRangeChanges = newPortRangeChangeRecorder(ctx.logger, hcParams.Unit.Tag(), ctx.modelType, machPorts, appPortRanges)

	ctx.secretChanges = newSecretsChangeRecorder(ctx.logger)

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

func NewMockUnitHookContext(mockUnit *mocks.MockHookUnit, modelType model.ModelType, leadership LeadershipContext) *HookContext {
	logger := loggo.GetLogger("test")
	return &HookContext{
		unit:              mockUnit,
		unitName:          mockUnit.Tag().Id(),
		logger:            logger,
		modelType:         modelType,
		LeadershipContext: leadership,
		portRangeChanges: newPortRangeChangeRecorder(logger, mockUnit.Tag(), modelType, nil,
			map[names.UnitTag]network.GroupedPortRanges{
				mockUnit.Tag(): {
					"": []network.PortRange{network.MustParsePortRange("666-888/tcp")},
				},
			},
		),
		secretChanges:          newSecretsChangeRecorder(logger),
		storageAttachmentCache: make(map[names.StorageTag]jujuc.ContextStorageAttachment),
	}
}

func NewMockUnitHookContextWithState(mockUnit *mocks.MockHookUnit, state *uniter.State) *HookContext {
	logger := loggo.GetLogger("test")
	return &HookContext{
		unitName:               mockUnit.Tag().Id(), //unitName used by the action finaliser method.
		unit:                   mockUnit,
		state:                  state,
		logger:                 logger,
		modelType:              model.IAAS,
		portRangeChanges:       newPortRangeChangeRecorder(logger, mockUnit.Tag(), model.IAAS, nil, nil),
		secretChanges:          newSecretsChangeRecorder(logger),
		storageAttachmentCache: make(map[names.StorageTag]jujuc.ContextStorageAttachment),
	}
}

func NewMockUnitHookContextWithStateAndStorage(unitName string, unit HookUnit, state State, storageTag names.StorageTag) *HookContext {
	logger := loggo.GetLogger("test")
	return &HookContext{
		unitName:               unit.Tag().Id(), //unitName used by the action finaliser method.
		unit:                   unit,
		state:                  state,
		logger:                 logger,
		portRangeChanges:       newPortRangeChangeRecorder(logger, names.NewUnitTag(unitName), model.IAAS, nil, nil),
		storageTag:             storageTag,
		storageAttachmentCache: make(map[names.StorageTag]jujuc.ContextStorageAttachment),
	}
}

// SetEnvironmentHookContextSecret exists purely to set the fields used in hookVars.
func SetEnvironmentHookContextSecret(
	context *HookContext, secretURI string, metadata map[string]jujuc.SecretMetadata, client SecretsAccessor, backend jujusecrets.BackendsClient,
) {
	context.secretURI = secretURI
	context.secretLabel = "label-" + secretURI
	context.secretRevision = 666
	context.secretsClient = client
	context.secretsBackend = backend
	context.secretMetadata = metadata
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

// SetEnvironmentHookContextStorage exists purely to set the fields used in hookVars.
// It makes no assumptions about the validity of context.
func SetEnvironmentHookContextStorage(context *HookContext, storageTag names.StorageTag) {
	context.storageTag = storageTag
}

// SetEnvironmentHookContextNotice exists purely to set the fields used in hookVars.
// It makes no assumptions about the validity of context.
func SetEnvironmentHookContextNotice(context *HookContext, workloadName, noticeID, noticeType, noticeKey string) {
	context.workloadName = workloadName
	context.noticeID = noticeID
	context.noticeType = noticeType
	context.noticeKey = noticeKey
}

// SetRelationBroken sets the relation as broken.
func SetRelationBroken(context jujuc.Context, relId int) {
	context.(*HookContext).relations[relId].broken = true
}

// RelationBroken returns the relation broken state.
func RelationBroken(context jujuc.Context, relId int) bool {
	return context.(*HookContext).relations[relId].broken
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

	State State
	Unit  HookUnit
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
		relationId:             -1,
		assignedMachineTag:     p.MachineTag,
		availabilityZone:       p.AvailZone,
		slaLevel:               p.SLALevel,
		principal:              p.UnitName,
		cloudAPIVersion:        "6.66",
		logger:                 loggo.GetLogger("test"),
		state:                  p.State,
		unit:                   p.Unit,
		storageAttachmentCache: make(map[names.StorageTag]jujuc.ContextStorageAttachment),
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

func (ctx *HookContext) PendingSecretRemoves() map[string]uniter.SecretDeleteArg {
	return ctx.secretChanges.pendingDeletes
}

func (ctx *HookContext) PendingSecretCreates() map[string]uniter.SecretCreateArg {
	return ctx.secretChanges.pendingCreates
}

func (ctx *HookContext) PendingSecretUpdates() map[string]uniter.SecretUpdateArg {
	return ctx.secretChanges.pendingUpdates
}

func (ctx *HookContext) SetPendingSecretCreates(in map[string]uniter.SecretCreateArg) {
	ctx.secretChanges.pendingCreates = in
}

func (ctx *HookContext) SetPendingSecretUpdates(in map[string]uniter.SecretUpdateArg) {
	ctx.secretChanges.pendingUpdates = in
}

func (ctx *HookContext) PendingSecretGrants() map[string]map[string]uniter.SecretGrantRevokeArgs {
	return ctx.secretChanges.pendingGrants
}

func (ctx *HookContext) PendingSecretRevokes() map[string][]uniter.SecretGrantRevokeArgs {
	return ctx.secretChanges.pendingRevokes
}

func (ctx *HookContext) PendingSecretTrackLatest() map[string]bool {
	return ctx.secretChanges.pendingTrackLatest
}
