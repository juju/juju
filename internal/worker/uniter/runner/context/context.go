// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/proxy"

	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/caas"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/application"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/quota"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/runner/context/payloads"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/version"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use xxx.
type logger interface{}

var _ logger = struct{}{}

// Context exposes hooks.Context, and additional methods needed by Runner.
type Context interface {
	jujuc.Context
	Id() string
	HookVars(
		paths Paths,
		remote bool,
		env Environmenter) ([]string, error)
	ActionData() (*ActionData, error)
	SetProcess(process HookProcess)
	HasExecutionSetUnitStatus() bool
	ResetExecutionSetUnitStatus()
	ModelType() model.ModelType

	Prepare() error
	Flush(badge string, failure error) error

	GetLogger(module string) loggo.Logger
}

// Paths exposes the paths needed by Context.
type Paths interface {
	// GetToolsDir returns the filesystem path to the dirctory containing
	// the hook tool symlinks.
	GetToolsDir() string

	// GetBaseDir returns the filesystem path to the directory in which
	// the charm is installed.
	GetBaseDir() string

	// GetCharmDir returns the filesystem path to the directory in which
	// the charm is installed.
	GetCharmDir() string

	// GetJujucServerSocket returns the path to the socket used by the hook tools
	// to communicate back to the executing uniter process. It might be a
	// filesystem path, or it might be abstract.
	GetJujucServerSocket(remote bool) sockets.Socket

	// GetJujucClientSocket returns the path to the socket used by the hook tools
	// to communicate back to the executing uniter process. It might be a
	// filesystem path, or it might be abstract.
	GetJujucClientSocket(remote bool) sockets.Socket

	// GetMetricsSpoolDir returns the path to a metrics spool dir, used
	// to store metrics recorded during a single hook run.
	GetMetricsSpoolDir() string

	// GetResourcesDir returns the filesystem path to the directory
	// containing resource data files.
	GetResourcesDir() string
}

// Clock defines the methods of the full clock.Clock that are needed here.
type Clock interface {
	// After waits for the duration to elapse and then sends the
	// current time on the returned channel.
	After(time.Duration) <-chan time.Time
}

var ErrIsNotLeader = errors.Errorf("this unit is not the leader")

// meterStatus describes the unit's meter status.
type meterStatus struct {
	code string
	info string
}

// HookProcess is an interface representing a process running a hook.
type HookProcess interface {
	Pid() int
	Kill() error
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/hookunit_mock.go github.com/juju/juju/internal/worker/uniter/runner/context HookUnit
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/internal/worker/uniter/runner/context State

// HookUnit represents the functions needed by a unit in a hook context to
// call into state.
type HookUnit interface {
	Application() (*uniter.Application, error)
	ApplicationName() string
	ConfigSettings() (charm.Settings, error)
	LogActionMessage(names.ActionTag, string) error
	Name() string
	NetworkInfo(bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error)
	RequestReboot() error
	SetUnitStatus(unitStatus status.Status, info string, data map[string]interface{}) error
	SetAgentStatus(agentStatus status.Status, info string, data map[string]interface{}) error
	State() (params.UnitStateResult, error)
	Tag() names.UnitTag
	UnitStatus() (params.StatusResult, error)
	CommitHookChanges(params.CommitHookChangesArgs) error
	PublicAddress() (string, error)
}

// State exposes required state functions needed by the HookContext.
type State interface {
	UnitStorageAttachments(unitTag names.UnitTag) ([]params.StorageAttachmentId, error)
	StorageAttachment(storageTag names.StorageTag, unitTag names.UnitTag) (params.StorageAttachment, error)
	GoalState() (application.GoalState, error)
	GetPodSpec(appName string) (string, error)
	GetRawK8sSpec(appName string) (string, error)
	CloudSpec() (*params.CloudSpec, error)
	ActionBegin(tag names.ActionTag) error
	ActionFinish(tag names.ActionTag, status string, results map[string]interface{}, message string) error
	UnitWorkloadVersion(tag names.UnitTag) (string, error)
	SetUnitWorkloadVersion(tag names.UnitTag, version string) error
	OpenedMachinePortRangesByEndpoint(machineTag names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error)
	OpenedPortRangesByEndpoint() (map[names.UnitTag]network.GroupedPortRanges, error)
}

// HookContext is the implementation of runner.Context.
type HookContext struct {
	*resources.ResourcesHookContext
	*payloads.PayloadsHookContext
	unit HookUnit

	// state is the handle to the uniter State so that HookContext can make
	// API calls on the state.
	// NOTE: We would like to be rid of the fake-remote-Unit and switch
	// over fully to API calls on State.  This adds that ability, but we're
	// not fully there yet.
	state State

	// secretsClient allows the context to access the secrets backend.
	secretsClient SecretsAccessor

	// secretsBackendGetter is used to get a client to access the secrets backend.
	secretsBackendGetter SecretsBackendGetter
	// secretsBackend is the secrets backend client, created only when needed.
	secretsBackend secrets.BackendsClient

	// LeadershipContext supplies several hooks.Context methods.
	LeadershipContext

	// principal is the unitName of the principal charm.
	principal string

	// privateAddress is the cached value of the unit's private
	// address.
	privateAddress string

	// publicAddress is the cached value of the unit's public
	// address.
	publicAddress string

	// availabilityZone is the cached value of the unit's
	// availability zone name.
	availabilityZone string

	// configSettings holds the application configuration.
	configSettings charm.Settings

	// goalState holds the goal state struct
	goalState application.GoalState

	// id identifies the context.
	id string

	hookName string

	// actionData contains the values relevant to the run of an Action:
	// its tag, its parameters, and its results.
	actionData *ActionData
	// actionDataMu protects against concurrent access to actionData.
	actionDataMu sync.Mutex

	// uuid is the universally unique identifier of the environment.
	uuid string

	// modelName is the human friendly name of the environment.
	modelName string

	// modelType
	modelType model.ModelType

	// unitName is the human friendly name of the local unit.
	unitName string

	// status is the status of the local unit.
	status *jujuc.StatusInfo

	// relationId identifies the relation for which a relation hook is
	// executing. If it is -1, the context is not running a relation hook;
	// otherwise, its value must be a valid key into the relations map.
	relationId int

	// remoteUnitName identifies the changing unit of the executing relation
	// hook. It will be empty if the context is not running a relation hook,
	// or if it is running a relation-broken hook.
	remoteUnitName string

	// remoteApplicationName identifies the application name in response to
	// relation-set --app.
	remoteApplicationName string

	// relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	relations map[int]*ContextRelation

	// departingUnitName identifies the unit that goes away from the relation.
	// It is only populated when running a RelationDeparted hook.
	departingUnitName string

	// apiAddrs contains the API server addresses.
	apiAddrs []string

	// legacyProxySettings are the current legacy proxy settings
	// that the uniter knows about.
	legacyProxySettings proxy.Settings

	// jujuProxySettings are the current juju proxy settings
	// that the uniter knows about.
	jujuProxySettings proxy.Settings

	// meterStatus is the status of the unit's metering.
	meterStatus *meterStatus

	// a helper for recording requests to open/close port ranges for this unit.
	portRangeChanges *portRangeChangeRecorder

	// assignedMachineTag contains the tag of the unit's assigned
	// machine.
	assignedMachineTag names.MachineTag

	// process is the process of the command that is being run in the local context,
	// like a juju-exec command or a hook
	process HookProcess

	// rebootPriority tells us when the hook wants to reboot. If rebootPriority
	// is hooks.RebootNow the hook will be killed and requeued.
	rebootPriority jujuc.RebootPriority

	// storageId is the tag of the storage instance associated
	// with the running hook.
	storageTag names.StorageTag

	// storageTags returns the tags of storage instances attached to the unit
	storageTags []names.StorageTag

	// storageAttachmentCache holds cached storage attachments so that hook
	// calls are consistent.
	storageAttachmentCache map[names.StorageTag]jujuc.ContextStorageAttachment

	// hasRunSetStatus is true if a call to the status-set was made during the
	// invocation of a hook.
	// This attribute is persisted to local uniter state at the end of the hook
	// execution so that the uniter can ultimately decide if it needs to update
	// a charm's workload status, or if the charm has already taken care of it.
	hasRunStatusSet bool

	// storageAddConstraints is a collection of storage constraints
	// keyed on storage name as specified in the charm.
	// This collection will be added to the unit on successful
	// hook run, so the actual add will happen in a flush.
	storageAddConstraints map[string][]params.StorageConstraints

	// clock is used for any time operations.
	clock Clock

	logger loggo.Logger

	// slaLevel contains the current SLA level.
	slaLevel string

	// The cloud specification
	cloudSpec *params.CloudSpec

	// The cloud API version, if available.
	cloudAPIVersion string

	// podSpecYaml is the pending pod spec to be committed.
	podSpecYaml *string

	// k8sRawSpecYaml is the pending raw k8s spec to be committed.
	k8sRawSpecYaml *string

	// A cached view of the unit's charm state that gets persisted by juju
	// once the context is flushed.
	cachedCharmState map[string]string

	// A flag that keeps track of whether the unit's state has been mutated.
	charmStateCacheDirty bool

	// workloadName is the name of the container which the hook is in relation to.
	workloadName string

	// noticeID is the Pebble notice ID associated with the hook.
	noticeID string

	// noticeType is the Pebble notice type associated with the hook.
	noticeType string

	// noticeKey is the Pebble notice key associated with the hook.
	noticeKey string

	// checkName is the Pebble check name associated with the hook.
	checkName string

	// baseUpgradeTarget is the base that the unit's machine is to be
	// updated to when Juju is issued the `upgrade-machine` command.
	baseUpgradeTarget string

	// secretURI is the reference to the secret relevant to the hook.
	secretURI string

	// secretRevision is the secret revision relevant to the hook.
	secretRevision int

	// secretLabel is the secret label to expose to the hook.
	secretLabel string

	// secretMetadata contains the metadata for secrets created by this charm.
	secretMetadata map[string]jujuc.SecretMetadata

	// secretChanges records changes to secrets during a hook execution.
	secretChanges *secretsChangeRecorder

	mu sync.Mutex
}

// GetLogger returns a Logger for the specified module.
func (ctx *HookContext) GetLogger(module string) loggo.Logger {
	return ctx.logger.Root().Child(module)
}

// GetCharmState returns a copy of the cached charm state.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (ctx *HookContext) GetCharmState() (map[string]string, error) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if err := ctx.ensureCharmStateLoaded(); err != nil {
		return nil, err
	}

	if len(ctx.cachedCharmState) == 0 {
		return nil, nil
	}

	retVal := make(map[string]string, len(ctx.cachedCharmState))
	for k, v := range ctx.cachedCharmState {
		retVal[k] = v
	}
	return retVal, nil
}

// GetCharmStateValue returns the value of the given key.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (ctx *HookContext) GetCharmStateValue(key string) (string, error) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if err := ctx.ensureCharmStateLoaded(); err != nil {
		return "", err
	}

	value, ok := ctx.cachedCharmState[key]
	if !ok {
		return "", errors.NotFoundf("%q", key)
	}
	return value, nil
}

// SetCharmStateValue sets the key/value pair provided in the cache.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (ctx *HookContext) SetCharmStateValue(key, value string) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if err := ctx.ensureCharmStateLoaded(); err != nil {
		return err
	}

	// Enforce fixed quota limit for key/value sizes. Performing this check
	// as early as possible allows us to provide feedback to charm authors
	// who might be tempted to exploit this feature for storing CLOBs/BLOBs.
	if err := quota.CheckTupleSize(key, value, quota.MaxCharmStateKeySize, quota.MaxCharmStateValueSize); err != nil {
		return errors.Trace(err)
	}

	curValue, exists := ctx.cachedCharmState[key]
	if exists && curValue == value {
		return nil // no-op
	}

	ctx.cachedCharmState[key] = value
	ctx.charmStateCacheDirty = true
	return nil
}

// DeleteCharmStateValue deletes the key/value pair for the given key from
// the cache.
// Implements jujuc.HookContext.unitCharmStateContext, part of runner.Context.
func (ctx *HookContext) DeleteCharmStateValue(key string) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if err := ctx.ensureCharmStateLoaded(); err != nil {
		return err
	}

	if _, exists := ctx.cachedCharmState[key]; !exists {
		return nil // no-op
	}

	delete(ctx.cachedCharmState, key)
	ctx.charmStateCacheDirty = true
	return nil
}

// ensureCharmStateLoaded retrieves and caches the unit's charm state from the
// controller. The caller of this method must be holding the ctx mutex.
func (ctx *HookContext) ensureCharmStateLoaded() error {
	// NOTE: Assuming lock to be held!
	if ctx.cachedCharmState != nil {
		return nil
	}

	// Load from controller
	var charmState map[string]string
	unitState, err := ctx.unit.State()
	if err != nil {
		return errors.Annotate(err, "loading unit state from database")
	}
	if unitState.CharmState == nil {
		charmState = make(map[string]string)
	} else {
		charmState = unitState.CharmState
	}

	ctx.cachedCharmState = charmState
	ctx.charmStateCacheDirty = false
	return nil
}

// RequestReboot will set the reboot flag to true on the machine agent
// Implements jujuc.HookContext.ContextInstance, part of runner.Context.
func (ctx *HookContext) RequestReboot(priority jujuc.RebootPriority) error {
	// Must set reboot priority first, because killing the hook
	// process will trigger the completion of the hook. If killing
	// the hook fails, then we can reset the priority.
	ctx.setRebootPriority(priority)

	var err error
	if priority == jujuc.RebootNow {
		// At this point, the hook should be running
		err = ctx.killCharmHook()
	}

	switch err {
	case nil, charmrunner.ErrNoProcess:
		// ErrNoProcess almost certainly means we are running in debug hooks
	default:
		ctx.setRebootPriority(jujuc.RebootSkip)
	}
	return err
}

func (ctx *HookContext) GetRebootPriority() jujuc.RebootPriority {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.rebootPriority
}

func (ctx *HookContext) setRebootPriority(priority jujuc.RebootPriority) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.rebootPriority = priority
}

func (ctx *HookContext) GetProcess() HookProcess {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.process
}

// SetProcess implements runner.Context.
func (ctx *HookContext) SetProcess(process HookProcess) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.process = process
}

// Id returns an integer which uniquely identifies the relation.
// Implements jujuc.HookContext.ContextRelation, part of runner.Context.
func (ctx *HookContext) Id() string {
	return ctx.id
}

// UnitName returns the executing unit's name.
// UnitName implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) UnitName() string {
	return ctx.unitName
}

// ModelType of the context we are running in.
// SetProcess implements runner.Context.
func (ctx *HookContext) ModelType() model.ModelType {
	return ctx.modelType
}

// UnitStatus will return the status for the current Unit.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (ctx *HookContext) UnitStatus() (*jujuc.StatusInfo, error) {
	if ctx.status == nil {
		var err error
		unitStatus, err := ctx.unit.UnitStatus()
		if err != nil {
			return nil, err
		}
		ctx.status = &jujuc.StatusInfo{
			Status: unitStatus.Status,
			Info:   unitStatus.Info,
			Data:   unitStatus.Data,
		}
	}
	return ctx.status, nil
}

// ApplicationStatus returns the status for the application and all the units on
// the application to which this context unit belongs, only if this unit is
// the leader.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (ctx *HookContext) ApplicationStatus() (jujuc.ApplicationStatusInfo, error) {
	var err error
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		return jujuc.ApplicationStatusInfo{}, ErrIsNotLeader
	}
	app, err := ctx.unit.Application()
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}
	appStatus, err := app.Status(ctx.unit.Name())
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}
	us := make([]jujuc.StatusInfo, len(appStatus.Units))
	i := 0
	for t, s := range appStatus.Units {
		us[i] = jujuc.StatusInfo{
			Tag:    t,
			Status: s.Status,
			Info:   s.Info,
			Data:   s.Data,
		}
		i++
	}
	return jujuc.ApplicationStatusInfo{
		Application: jujuc.StatusInfo{
			Tag:    app.Tag().String(),
			Status: appStatus.Application.Status,
			Info:   appStatus.Application.Info,
			Data:   appStatus.Application.Data,
		},
		Units: us,
	}, nil
}

// SetUnitStatus will set the given status for this unit.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (ctx *HookContext) SetUnitStatus(unitStatus jujuc.StatusInfo) error {
	ctx.hasRunStatusSet = true
	ctx.logger.Tracef("[WORKLOAD-STATUS] %s: %s", unitStatus.Status, unitStatus.Info)
	return ctx.unit.SetUnitStatus(
		status.Status(unitStatus.Status),
		unitStatus.Info,
		unitStatus.Data,
	)
}

// SetAgentStatus will set the given status for this unit's agent.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (ctx *HookContext) SetAgentStatus(agentStatus jujuc.StatusInfo) error {
	ctx.logger.Tracef("[AGENT-STATUS] %s: %s", agentStatus.Status, agentStatus.Info)
	return ctx.unit.SetAgentStatus(
		status.Status(agentStatus.Status),
		agentStatus.Info,
		agentStatus.Data,
	)
}

// SetApplicationStatus will set the given status to the application to which this
// unit's belong, only if this unit is the leader.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (ctx *HookContext) SetApplicationStatus(applicationStatus jujuc.StatusInfo) error {
	ctx.logger.Tracef("[APPLICATION-STATUS] %s: %s", applicationStatus.Status, applicationStatus.Info)
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		return ErrIsNotLeader
	}

	app, err := ctx.unit.Application()
	if err != nil {
		return errors.Trace(err)
	}
	return app.SetStatus(
		ctx.unit.Name(),
		status.Status(applicationStatus.Status),
		applicationStatus.Info,
		applicationStatus.Data,
	)
}

// HasExecutionSetUnitStatus implements runner.Context.
func (ctx *HookContext) HasExecutionSetUnitStatus() bool {
	return ctx.hasRunStatusSet
}

// ResetExecutionSetUnitStatus implements runner.Context.
func (ctx *HookContext) ResetExecutionSetUnitStatus() {
	ctx.hasRunStatusSet = false
}

// PublicAddress fetches the executing unit's public address if it has
// not yet been retrieved.
// The cached value is returned, or an error if it is not available.
func (ctx *HookContext) PublicAddress() (string, error) {
	if ctx.publicAddress == "" {
		var err error
		if ctx.publicAddress, err = ctx.unit.PublicAddress(); err != nil && !params.IsCodeNoAddressSet(err) {
			return "", errors.Trace(err)
		}
	}

	if ctx.publicAddress == "" {
		return "", errors.NotFoundf("public address")
	}
	return ctx.publicAddress, nil
}

// PrivateAddress returns the executing unit's private address or an
// error if it is not available.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) PrivateAddress() (string, error) {
	if ctx.privateAddress == "" {
		return "", errors.NotFoundf("private address")
	}
	return ctx.privateAddress, nil
}

// AvailabilityZone returns the executing unit's availability zone or an error
// if it was not found (or is not available).
// Implements jujuc.HookContext.ContextInstance, part of runner.Context.
func (ctx *HookContext) AvailabilityZone() (string, error) {
	if ctx.availabilityZone == "" {
		return "", errors.NotFoundf("availability zone")
	}
	return ctx.availabilityZone, nil
}

// StorageTags returns a list of tags for storage instances
// attached to the unit or an error if they are not available.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) StorageTags() ([]names.StorageTag, error) {
	// Comparing to nil on purpose here to cache an empty slice.
	if ctx.storageTags != nil {
		return ctx.storageTags, nil
	}
	attachmentIds, err := ctx.state.UnitStorageAttachments(ctx.unit.Tag())
	if err != nil {
		return nil, err
	}
	// N.B. zero-length non-nil slice on purpose.
	ctx.storageTags = make([]names.StorageTag, 0)
	for _, attachmentId := range attachmentIds {
		storageTag, err := names.ParseStorageTag(attachmentId.StorageTag)
		if err != nil {
			return nil, err
		}
		ctx.storageTags = append(ctx.storageTags, storageTag)
	}
	return ctx.storageTags, nil
}

// HookStorage returns the storage attachment associated
// the executing hook if it was found, and an error if it
// was not found or is not available.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) HookStorage() (jujuc.ContextStorageAttachment, error) {
	emptyTag := names.StorageTag{}
	if ctx.storageTag == emptyTag {
		return nil, errors.NotFound
	}
	return ctx.Storage(ctx.storageTag)
}

// Storage returns the ContextStorageAttachment with the supplied
// tag if it was found, and an error if it was not found or is not
// available to the context.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, error) {
	if ctxStorageAttachment, ok := ctx.storageAttachmentCache[tag]; ok {
		return ctxStorageAttachment, nil
	}
	attachment, err := ctx.state.StorageAttachment(tag, ctx.unit.Tag())
	if err != nil {
		return nil, err
	}
	ctxStorageAttachment := &contextStorage{
		tag:      tag,
		kind:     storage.StorageKind(attachment.Kind),
		location: attachment.Location,
	}
	ctx.storageAttachmentCache[tag] = ctxStorageAttachment
	return ctxStorageAttachment, nil
}

// AddUnitStorage saves storage constraints in the context.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) AddUnitStorage(cons map[string]params.StorageConstraints) error {
	// All storage constraints are accumulated before context is flushed.
	if ctx.storageAddConstraints == nil {
		ctx.storageAddConstraints = make(
			map[string][]params.StorageConstraints,
			len(cons))
	}
	for storage, newConstraints := range cons {
		// Multiple calls for the same storage are accumulated as well.
		ctx.storageAddConstraints[storage] = append(
			ctx.storageAddConstraints[storage],
			newConstraints)
	}
	return nil
}

// OpenPortRange marks the supplied port range for opening.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) OpenPortRange(endpointName string, portRange network.PortRange) error {
	return ctx.portRangeChanges.OpenPortRange(endpointName, portRange)
}

// ClosePortRange ensures the supplied port range is closed even when
// the executing unit's application is exposed (unless it is opened
// separately by a co- located unit).
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) ClosePortRange(endpointName string, portRange network.PortRange) error {
	return ctx.portRangeChanges.ClosePortRange(endpointName, portRange)
}

// OpenedPortRanges returns all port ranges currently opened by this
// unit on its assigned machine grouped by endpoint.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) OpenedPortRanges() network.GroupedPortRanges {
	return ctx.portRangeChanges.OpenedUnitPortRanges()
}

// ConfigSettings returns the current application configuration of the executing unit.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) ConfigSettings() (charm.Settings, error) {
	if ctx.configSettings == nil {
		var err error
		ctx.configSettings, err = ctx.unit.ConfigSettings()
		if err != nil {
			return nil, err
		}
	}
	result := charm.Settings{}
	for name, value := range ctx.configSettings {
		result[name] = value
	}
	return result, nil
}

func (ctx *HookContext) getSecretsBackend() (secrets.BackendsClient, error) {
	if ctx.secretsBackend != nil {
		return ctx.secretsBackend, nil
	}
	var err error
	ctx.secretsBackend, err = ctx.secretsBackendGetter()
	if err != nil {
		return nil, err
	}
	return ctx.secretsBackend, nil
}

func (ctx *HookContext) lookupOwnedSecretURIByLabel(label string) (*coresecrets.URI, error) {
	mds, err := ctx.SecretMetadata()
	if err != nil {
		return nil, err
	}
	for ID, md := range mds {
		if md.Label == label && md.Owner.Id() == ctx.unit.Tag().Id() {
			return &coresecrets.URI{ID: ID}, nil
		}
	}
	for _, md := range ctx.secretChanges.pendingCreates {
		// Check if we have any pending create changes.
		if md.Label == nil || md.URI == nil {
			continue
		}
		if *md.Label == label {
			return md.URI, nil
		}
	}
	for _, md := range ctx.secretChanges.pendingUpdates {
		// Check if we have any pending label update changes.
		if md.Label == nil || md.URI == nil {
			continue
		}
		if *md.Label == label {
			return md.URI, nil
		}
	}
	return nil, errors.NotFoundf("secret owned by %q with label %q", ctx.unitName, label)
}

// GetSecret returns the value of the specified secret.
func (ctx *HookContext) GetSecret(uri *coresecrets.URI, label string, refresh, peek bool) (coresecrets.SecretValue, error) {
	if uri == nil && label == "" {
		return nil, errors.NotValidf("empty URI and label")
	}
	if uri != nil {
		if v, got := ctx.getPendingSecretValue(uri, label, refresh, peek); got {
			return v, nil
		}
	}
	if label != "" {
		if v, got := ctx.getPendingSecretValue(nil, label, refresh, peek); got {
			return v, nil
		}
	}
	if uri == nil && label != "" {
		// try to resolve label to URI by looking up owned secrets.
		ownedSecretURI, err := ctx.lookupOwnedSecretURIByLabel(label)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, err
		}
		if ownedSecretURI != nil {
			// Found owned secret, no need label anymore.
			uri = ownedSecretURI
			label = ""
		}
	}
	backend, err := ctx.getSecretsBackend()
	if err != nil {
		return nil, err
	}
	v, err := backend.GetContent(uri, label, refresh, peek)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (ctx *HookContext) getPendingSecretValue(uri *coresecrets.URI, label string, refresh, peek bool) (coresecrets.SecretValue, bool) {
	if uri == nil && label == "" {
		return nil, false
	}
	for i, v := range ctx.secretChanges.pendingCreates {
		if uri != nil && v.URI != nil && v.URI.ID == uri.ID {
			if label != "" {
				pending := ctx.secretChanges.pendingCreates[i]
				pending.Label = &label
				ctx.secretChanges.pendingCreates[i] = pending
			}
			// The initial value of the secret is not stored in the database yet.
			return v.Value, true
		}
		if label != "" && v.Label != nil && label == *v.Label {
			// The initial value of the secret is not stored in the database yet.
			return v.Value, true
		}
	}
	if !refresh && !peek {
		return nil, false
	}

	for i, v := range ctx.secretChanges.pendingUpdates {
		if uri != nil && v.URI != nil && v.URI.ID == uri.ID {
			if label != "" {
				pending := ctx.secretChanges.pendingUpdates[i]
				pending.Label = &label
				ctx.secretChanges.pendingUpdates[i] = pending
			}
			if refresh {
				ctx.secretChanges.pendingTrackLatest[v.URI.ID] = true
			}
			// The new value of the secret is going to be updated to the database.
			return v.Value, v.Value != nil && !v.Value.IsEmpty()
		}
		if label != "" && v.Label != nil && label == *v.Label {
			if refresh {
				ctx.secretChanges.pendingTrackLatest[v.URI.ID] = true
			}
			// The new value of the secret is going to be updated to the database.
			return v.Value, v.Value != nil && !v.Value.IsEmpty()
		}
	}
	return nil, false
}

// CreateSecret creates a secret with the specified data.
func (ctx *HookContext) CreateSecret(args *jujuc.SecretCreateArgs) (*coresecrets.URI, error) {
	if args.OwnerTag.Kind() == names.ApplicationTagKind {
		isLeader, err := ctx.IsLeader()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return nil, ErrIsNotLeader
		}
	}
	if args.Value == nil || args.Value.IsEmpty() {
		return nil, errors.NotValidf("empty secrte content")
	}
	checksum, err := args.Value.Checksum()
	if err != nil {
		return nil, errors.Annotate(err, "calculating secret checksum")
	}
	uris, err := ctx.secretsClient.CreateSecretURIs(1)
	if err != nil {
		return nil, errors.Trace(err)
	}
	err = ctx.secretChanges.create(uniter.SecretCreateArg{
		SecretUpsertArg: uniter.SecretUpsertArg{
			URI:          uris[0],
			RotatePolicy: args.RotatePolicy,
			ExpireTime:   args.ExpireTime,
			Description:  args.Description,
			Label:        args.Label,
			Value:        args.Value,
			Checksum:     checksum,
		},
		OwnerTag: args.OwnerTag,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return uris[0], nil
}

// UpdateSecret creates a secret with the specified data.
func (ctx *HookContext) UpdateSecret(uri *coresecrets.URI, args *jujuc.SecretUpdateArgs) error {
	md, knowSecret := ctx.secretMetadata[uri.ID]
	if knowSecret && md.Owner.Kind() == names.ApplicationTagKind {
		isLeader, err := ctx.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	updateArg := uniter.SecretUpdateArg{
		SecretUpsertArg: uniter.SecretUpsertArg{
			URI:          uri,
			RotatePolicy: args.RotatePolicy,
			ExpireTime:   args.ExpireTime,
			Description:  args.Description,
			Label:        args.Label,
		},
		CurrentRevision: md.LatestRevision,
	}
	if args.Value != nil && !args.Value.IsEmpty() {
		checksum, err := args.Value.Checksum()
		if err != nil {
			return errors.Annotate(err, "calculating secret checksum")
		}
		if !knowSecret || md.LatestChecksum != checksum {
			updateArg.Value = args.Value
			updateArg.Checksum = checksum
		}
	}
	if args.RotatePolicy == nil && args.Description == nil && args.ExpireTime == nil &&
		args.Label == nil && updateArg.Value == nil {
		return nil
	}

	ctx.secretChanges.update(updateArg)
	return nil
}

// RemoveSecret removes a secret with the specified uri.
func (ctx *HookContext) RemoveSecret(uri *coresecrets.URI, revision *int) error {
	md, ok := ctx.secretMetadata[uri.ID]
	if ok && md.Owner.Kind() == names.ApplicationTagKind {
		isLeader, err := ctx.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	ctx.secretChanges.remove(uri, revision)
	return nil
}

// SecretMetadata gets the secret ids and their labels and latest revisions created by the charm.
// The result includes any pending updates.
func (ctx *HookContext) SecretMetadata() (map[string]jujuc.SecretMetadata, error) {
	result := make(map[string]jujuc.SecretMetadata)
	for _, c := range ctx.secretChanges.pendingCreates {
		md := jujuc.SecretMetadata{
			Owner:          c.OwnerTag,
			LatestRevision: 1,
			LatestChecksum: c.Checksum,
		}
		if c.Label != nil {
			md.Label = *c.Label
		}
		if c.Description != nil {
			md.Description = *c.Description
		}
		if c.RotatePolicy != nil {
			md.RotatePolicy = *c.RotatePolicy
		}
		if c.ExpireTime != nil {
			md.LatestExpireTime = c.ExpireTime
		}
		result[c.URI.ID] = md
	}
	for id, v := range ctx.secretMetadata {
		if _, ok := ctx.secretChanges.pendingDeletes[id]; ok {
			continue
		}
		if u, ok := ctx.secretChanges.pendingUpdates[id]; ok {
			if u.Label != nil {
				v.Label = *u.Label
			}
			if u.Description != nil {
				v.Description = *u.Description
			}
			if u.RotatePolicy != nil {
				v.RotatePolicy = *u.RotatePolicy
			}
			if u.ExpireTime != nil {
				v.LatestExpireTime = u.ExpireTime
			}
			v.LatestChecksum = u.Checksum
		}
		result[id] = v
	}
	for k, v := range result {
		uri := &coresecrets.URI{ID: k}
		var err error
		if v.Access, err = ctx.secretChanges.secretGrantInfo(uri, v.Access...); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}

// GrantSecret grants access to a specified secret.
func (ctx *HookContext) GrantSecret(uri *coresecrets.URI, arg *jujuc.SecretGrantRevokeArgs) error {
	secretMetadata, err := ctx.SecretMetadata()
	if err != nil {
		return errors.Trace(err)
	}
	md, ok := secretMetadata[uri.ID]
	if !ok {
		return errors.NotFoundf("secret %q", uri.ID)
	}
	if md.Owner.Kind() == names.ApplicationTagKind {
		isLeader, err := ctx.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	uniterArg := uniter.SecretGrantRevokeArgs{
		URI:             uri,
		ApplicationName: arg.ApplicationName,
		UnitName:        arg.UnitName,
		RelationKey:     arg.RelationKey,
		Role:            coresecrets.RoleView,
	}
	params := uniterArg.ToParams()
	if len(params.SubjectTags) != 1 {
		return errors.NewNotValid(nil, fmt.Sprintf("expected only 1 subject, got %d", len(params.SubjectTags)))
	}
	subjectTag, err := names.ParseTag(params.SubjectTags[0])
	if err != nil {
		return errors.Trace(err)
	}
	for _, g := range md.Access {
		if params.ScopeTag != g.Scope || params.Role != string(g.Role) {
			continue
		}
		existingTargetTag, err := names.ParseTag(g.Target)
		if err != nil {
			return errors.Trace(err)
		}
		if existingTargetTag.String() == subjectTag.String() {
			// No ops.
			return nil
		}
		if existingTargetTag.Kind() == names.ApplicationTagKind {
			// We haven already grant in application level, so no ops for any unit level grant.
			return nil
		}
		if subjectTag.Kind() == names.ApplicationTagKind {
			return errors.NewNotValid(nil, "any unit level grants need to be revoked before granting access to the corresponding application")
		}
	}
	ctx.secretChanges.grant(uniterArg)
	return nil
}

// RevokeSecret revokes access to a specified secret.
func (ctx *HookContext) RevokeSecret(uri *coresecrets.URI, args *jujuc.SecretGrantRevokeArgs) error {
	md, ok := ctx.secretMetadata[uri.ID]
	if ok && md.Owner.Kind() == names.ApplicationTagKind {
		isLeader, err := ctx.IsLeader()
		if err != nil {
			return errors.Annotatef(err, "cannot determine leadership")
		}
		if !isLeader {
			return ErrIsNotLeader
		}
	}
	ctx.secretChanges.revoke(uniter.SecretGrantRevokeArgs{
		URI:             uri,
		ApplicationName: args.ApplicationName,
		UnitName:        args.UnitName,
		RelationKey:     args.RelationKey,
	})
	return nil
}

// GoalState returns the goal state for the current unit.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) GoalState() (*application.GoalState, error) {
	var err error
	ctx.goalState, err = ctx.state.GoalState()
	if err != nil {
		return nil, err
	}

	return &ctx.goalState, nil
}

// SetPodSpec sets the podspec for the unit's application.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) SetPodSpec(specYaml string) error {
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		ctx.logger.Errorf("%q is not the leader but is setting application k8s spec", ctx.unitName)
		return ErrIsNotLeader
	}
	_, err = k8sspecs.ParsePodSpec(specYaml)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.podSpecYaml = &specYaml
	return nil
}

// SetRawK8sSpec sets the raw k8s spec for the unit's application.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) SetRawK8sSpec(specYaml string) error {
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		ctx.logger.Errorf("%q is not the leader but is setting application raw k8s spec", ctx.unitName)
		return ErrIsNotLeader
	}
	_, err = k8sspecs.ParseRawK8sSpec(specYaml)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.k8sRawSpecYaml = &specYaml
	return nil
}

// GetPodSpec returns the k8s spec for the unit's application.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) GetPodSpec() (string, error) {
	appName := ctx.unit.ApplicationName()
	return ctx.state.GetPodSpec(appName)
}

// GetRawK8sSpec returns the raw k8s spec for the unit's application.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) GetRawK8sSpec() (string, error) {
	appName := ctx.unit.ApplicationName()
	return ctx.state.GetRawK8sSpec(appName)
}

// CloudSpec return the cloud specification for the running unit's model.
// Implements jujuc.HookContext.ContextUnit, part of runner.Context.
func (ctx *HookContext) CloudSpec() (*params.CloudSpec, error) {
	var err error
	ctx.cloudSpec, err = ctx.state.CloudSpec()
	if err != nil {
		return nil, err
	}
	return ctx.cloudSpec, nil
}

// ActionParams simply returns the arguments to the Action.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (ctx *HookContext) ActionParams() (map[string]interface{}, error) {
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	if ctx.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return ctx.actionData.Params, nil
}

// LogActionMessage logs a progress message for the Action.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (ctx *HookContext) LogActionMessage(message string) error {
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	return ctx.unit.LogActionMessage(ctx.actionData.Tag, message)
}

// SetActionMessage sets a message for the Action, usually an error message.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (ctx *HookContext) SetActionMessage(message string) error {
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	ctx.actionData.ResultsMessage = message
	return nil
}

// SetActionFailed sets the fail state of the action.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (ctx *HookContext) SetActionFailed() error {
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	ctx.actionData.Failed = true
	return nil
}

// UpdateActionResults inserts new values for use with action-set and
// action-fail.  The results struct will be delivered to the controller
// upon completion of the Action.  It returns an error if not called on an
// Action-containing HookContext.
// Implements jujuc.ActionHookContext.actionHookContext, part of runner.Context.
func (ctx *HookContext) UpdateActionResults(keys []string, value interface{}) error {
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	addValueToMap(keys, value, ctx.actionData.ResultsMap)
	return nil
}

// HookRelation returns the ContextRelation associated with the executing
// hook if it was found, or an error if it was not found (or is not available).
// Implements jujuc.RelationHookContext.relationHookContext, part of runner.Context.
func (ctx *HookContext) HookRelation() (jujuc.ContextRelation, error) {
	return ctx.Relation(ctx.relationId)
}

// RemoteUnitName returns the name of the remote unit the hook execution
// is associated with if it was found, and an error if it was not found or is not
// available.
// Implements jujuc.RelationHookContext.relationHookContext, part of runner.Context.
func (ctx *HookContext) RemoteUnitName() (string, error) {
	if ctx.remoteUnitName == "" {
		return "", errors.NotFoundf("remote unit")
	}
	return ctx.remoteUnitName, nil
}

// RemoteApplicationName returns the name of the remote application the hook execution
// is associated with if it was found, and an error if it was not found or is not
// available.
// Implements jujuc.RelationHookContext.relationHookContext, part of runner.Context.
func (ctx *HookContext) RemoteApplicationName() (string, error) {
	if ctx.remoteApplicationName == "" {
		return "", errors.NotFoundf("saas application")
	}
	return ctx.remoteApplicationName, nil
}

// Relation returns the relation with the supplied id if it was found, and
// an error if it was not found or is not available.
// Implements jujuc.HookContext.ContextRelations, part of runner.Context.
func (ctx *HookContext) Relation(id int) (jujuc.ContextRelation, error) {
	r, found := ctx.relations[id]
	if !found {
		return nil, errors.NotFoundf("relation")
	}
	return r, nil
}

// RelationIds returns the ids of all relations the executing unit is
// currently participating in or an error if they are not available.
// Implements jujuc.HookContext.ContextRelations, part of runner.Context.
func (ctx *HookContext) RelationIds() ([]int, error) {
	ids := []int{}
	for id, r := range ctx.relations {
		if r.broken {
			ctx.logger.Debugf("relation %d is broken, excluding from relations-ids", id)
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// AddMetric adds metrics to the hook context.
// Implements jujuc.HookContext.ContextMetrics, part of runner.Context.
func (ctx *HookContext) AddMetric(key, value string, created time.Time) error {
	return errors.New("metrics not allowed in this context")
}

// AddMetricLabels adds metrics with labels to the hook context.
// Implements jujuc.HookContext.ContextMetrics, part of runner.Context.
func (ctx *HookContext) AddMetricLabels(key, value string, created time.Time, labels map[string]string) error {
	return errors.New("metrics not allowed in this context")
}

// ActionData returns the context's internal action data. It's meant to be
// transitory; it exists to allow uniter and runner code to keep working as
// it did; it should be considered deprecated, and not used by new clients.
// Implements runner.Context.
func (ctx *HookContext) ActionData() (*ActionData, error) {
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	if ctx.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return ctx.actionData, nil
}

// HookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into context.
// Implements runner.Context.
func (ctx *HookContext) HookVars(
	paths Paths,
	remote bool,
	env Environmenter,
) ([]string, error) {
	vars := ctx.legacyProxySettings.AsEnvironmentValues()
	vars = append(vars, ContextDependentEnvVars(env)...)

	// TODO(thumper): as work on proxies progress, there will be additional
	// proxy settings to be added.
	vars = append(vars,
		"CHARM_DIR="+paths.GetCharmDir(), // legacy, embarrassing
		"JUJU_CHARM_DIR="+paths.GetCharmDir(),
		"JUJU_CONTEXT_ID="+ctx.id,
		"JUJU_HOOK_NAME="+ctx.hookName,
		"JUJU_AGENT_SOCKET_ADDRESS="+paths.GetJujucClientSocket(remote).Address,
		"JUJU_AGENT_SOCKET_NETWORK="+paths.GetJujucClientSocket(remote).Network,
		"JUJU_UNIT_NAME="+ctx.unitName,
		"JUJU_MODEL_UUID="+ctx.uuid,
		"JUJU_MODEL_NAME="+ctx.modelName,
		"JUJU_API_ADDRESSES="+strings.Join(ctx.apiAddrs, " "),
		"JUJU_SLA="+ctx.slaLevel,
		"JUJU_MACHINE_ID="+ctx.assignedMachineTag.Id(),
		"JUJU_PRINCIPAL_UNIT="+ctx.principal,
		"JUJU_AVAILABILITY_ZONE="+ctx.availabilityZone,
		"JUJU_VERSION="+version.Current.String(),
		"CLOUD_API_VERSION="+ctx.cloudAPIVersion,
		// Some of these will be empty, but that is fine, better
		// to explicitly export them as empty.
		"JUJU_CHARM_HTTP_PROXY="+ctx.jujuProxySettings.Http,
		"JUJU_CHARM_HTTPS_PROXY="+ctx.jujuProxySettings.Https,
		"JUJU_CHARM_FTP_PROXY="+ctx.jujuProxySettings.Ftp,
		"JUJU_CHARM_NO_PROXY="+ctx.jujuProxySettings.NoProxy,
	)
	if remote {
		vars = append(vars,
			"JUJU_AGENT_CA_CERT="+path.Join(paths.GetBaseDir(), caas.CACertFile),
		)
	}
	if ctx.meterStatus != nil {
		vars = append(vars,
			"JUJU_METER_STATUS="+ctx.meterStatus.code,
			"JUJU_METER_INFO="+ctx.meterStatus.info,
		)

	}
	if r, err := ctx.HookRelation(); err == nil {
		vars = append(vars,
			"JUJU_RELATION="+r.Name(),
			"JUJU_RELATION_ID="+r.FakeId(),
			"JUJU_REMOTE_UNIT="+ctx.remoteUnitName,
			"JUJU_REMOTE_APP="+ctx.remoteApplicationName,
		)

		if ctx.departingUnitName != "" {
			vars = append(vars,
				"JUJU_DEPARTING_UNIT="+ctx.departingUnitName,
			)
		}
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if ctx.actionData != nil {
		vars = append(vars,
			"JUJU_ACTION_NAME="+ctx.actionData.Name,
			"JUJU_ACTION_UUID="+ctx.actionData.Tag.Id(),
			"JUJU_ACTION_TAG="+ctx.actionData.Tag.String(),
		)
	}
	if ctx.workloadName != "" {
		vars = append(vars, "JUJU_WORKLOAD_NAME="+ctx.workloadName)
		if ctx.noticeID != "" {
			vars = append(vars,
				"JUJU_NOTICE_ID="+ctx.noticeID,
				"JUJU_NOTICE_TYPE="+ctx.noticeType,
				"JUJU_NOTICE_KEY="+ctx.noticeKey,
			)
		}
		if ctx.checkName != "" {
			vars = append(vars, "JUJU_PEBBLE_CHECK_NAME="+ctx.checkName)
		}
	}

	if ctx.baseUpgradeTarget != "" {
		// We need to set both the base and the series for the hook. This until
		// we migrate everything to use base.
		b, err := corebase.ParseBaseFromString(ctx.baseUpgradeTarget)
		if err != nil {
			return nil, errors.Trace(err)
		}
		s, err := corebase.GetSeriesFromBase(b)
		if err != nil {
			return nil, errors.Trace(err)
		}

		vars = append(vars,
			"JUJU_TARGET_BASE="+ctx.baseUpgradeTarget,
			"JUJU_TARGET_SERIES="+s,
		)
	}

	if ctx.secretURI != "" {
		vars = append(vars,
			"JUJU_SECRET_ID="+ctx.secretURI,
			"JUJU_SECRET_LABEL="+ctx.secretLabel,
		)
		if ctx.secretRevision > 0 {
			vars = append(vars,
				"JUJU_SECRET_REVISION="+strconv.Itoa(ctx.secretRevision),
			)
		}
	}

	if storage, err := ctx.HookStorage(); err == nil {
		vars = append(vars,
			"JUJU_STORAGE_ID="+storage.Tag().Id(),
			"JUJU_STORAGE_LOCATION="+storage.Location(),
			"JUJU_STORAGE_KIND="+storage.Kind().String(),
		)
	} else if !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.NotProvisioned) {
		return nil, errors.Trace(err)
	}

	return append(vars, OSDependentEnvVars(paths, env)...), nil
}

func (ctx *HookContext) handleReboot(ctxErr error) error {
	ctx.logger.Tracef("checking for reboot request")
	rebootPriority := ctx.GetRebootPriority()
	switch rebootPriority {
	case jujuc.RebootSkip:
		return ctxErr
	case jujuc.RebootAfterHook:
		// Reboot should only happen after hook finished successfully.
		if ctxErr != nil {
			return ctxErr
		}
		ctxErr = ErrReboot
	case jujuc.RebootNow:
		ctxErr = ErrRequeueAndReboot
	}

	// Do a best-effort attempt to set the unit agent status; we don't care
	// if it fails as we will request a reboot anyway.
	if err := ctx.unit.SetAgentStatus(status.Rebooting, "", nil); err != nil {
		ctx.logger.Errorf("updating agent status: %v", err)
	}

	if err := ctx.unit.RequestReboot(); err != nil {
		return err
	}

	return ctxErr
}

// Prepare implements the runner.Context interface.
func (ctx *HookContext) Prepare() error {
	if ctx.actionData != nil {
		err := ctx.state.ActionBegin(ctx.actionData.Tag)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Flush implements the runner.Context interface.
func (ctx *HookContext) Flush(process string, ctxErr error) error {
	// Apply the changes if no error reported while the hook was executing.
	var flushErr error
	if ctxErr == nil {
		flushErr = ctx.doFlush(process)
	}

	if ctx.actionData != nil {
		// While an Action is executing, errors may happen as part of
		// its normal flow. We pass both potential action errors
		// (ctxErr) and any flush errors to finalizeAction helper
		// which will figure out if an error needs to be returned back
		// to the uniter.
		return ctx.finalizeAction(ctxErr, flushErr)
	}

	// TODO(gsamfira): Just for now, reboot will not be supported in actions.
	if ctxErr == nil {
		ctxErr = flushErr
	}
	return ctx.handleReboot(ctxErr)
}

func (ctx *HookContext) doFlush(process string) error {
	b := uniter.NewCommitHookParamsBuilder(ctx.unit.Tag())

	// When processing config changed hooks we need to ensure that the
	// relation settings for the unit endpoints are up to date after
	// potential changes to already bound endpoints.
	if process == string(hooks.ConfigChanged) {
		b.UpdateNetworkInfo()
	}

	if ctx.charmStateCacheDirty {
		b.UpdateCharmState(ctx.cachedCharmState)
	}

	for _, rctx := range ctx.relations {
		unitSettings, appSettings := rctx.FinalSettings()
		if len(unitSettings)+len(appSettings) == 0 {
			continue // no settings need updating
		}
		b.UpdateRelationUnitSettings(rctx.RelationTag().String(), unitSettings, appSettings)
	}

	for endpointName, portRanges := range ctx.portRangeChanges.pendingOpenRanges {
		for _, pr := range portRanges {
			b.OpenPortRange(endpointName, pr)
		}
	}
	for endpointName, portRanges := range ctx.portRangeChanges.pendingCloseRanges {
		for _, pr := range portRanges {
			b.ClosePortRange(endpointName, pr)
		}
	}

	if len(ctx.storageAddConstraints) > 0 {
		b.AddStorage(ctx.storageAddConstraints)
	}

	// Before saving the secret metadata to Juju, save the content to an external
	// backend (if configured) - we need the backend id to send to Juju.
	// If the flush to Juju fails, we'll delete the external content.
	var secretsBackend secrets.BackendsClient
	if ctx.secretChanges.haveContentUpdates() {
		var err error
		secretsBackend, err = ctx.getSecretsBackend()
		if err != nil {
			return errors.Trace(err)
		}
	}

	var (
		cleanups           []coresecrets.ValueRef
		pendingCreates     []uniter.SecretCreateArg
		pendingUpdates     []uniter.SecretUpsertArg
		pendingDeletes     []uniter.SecretDeleteArg
		pendingGrants      []uniter.SecretGrantRevokeArgs
		pendingRevokes     []uniter.SecretGrantRevokeArgs
		pendingTrackLatest []string
	)
	for _, c := range ctx.secretChanges.pendingCreates {
		ref, err := secretsBackend.SaveContent(c.URI, 1, c.Value)
		if errors.Is(err, errors.NotSupported) {
			pendingCreates = append(pendingCreates, c)
			continue
		}
		if err != nil {
			return errors.Annotatef(err, "saving content for secret %q", c.URI.ID)
		}
		cleanups = append(cleanups, ref)
		c.ValueRef = &ref
		c.Value = nil
		pendingCreates = append(pendingCreates, c)
	}
	for _, u := range ctx.secretChanges.pendingUpdates {
		// Juju checks that the current revision is stable when updating metadata so it's
		// safe to increment here knowing the same value will be saved in Juju.
		if u.Value == nil || u.Value.IsEmpty() {
			pendingUpdates = append(pendingUpdates, u.SecretUpsertArg)
			continue
		}
		ref, err := secretsBackend.SaveContent(u.URI, u.CurrentRevision+1, u.Value)
		if errors.Is(err, errors.NotSupported) {
			pendingUpdates = append(pendingUpdates, u.SecretUpsertArg)
			continue
		}
		if err != nil {
			return errors.Annotatef(err, "saving content for secret %q", u.URI.ID)
		}
		cleanups = append(cleanups, ref)
		u.ValueRef = &ref
		u.Value = nil
		pendingUpdates = append(pendingUpdates, u.SecretUpsertArg)
	}

	for _, d := range ctx.secretChanges.pendingDeletes {
		pendingDeletes = append(pendingDeletes, d)
		md, ok := ctx.secretMetadata[d.URI.ID]
		if !ok {
			continue
		}
		var toDelete []int
		if d.Revision == nil {
			toDelete = md.Revisions
		} else {
			toDelete = []int{*d.Revision}
		}
		ctx.logger.Debugf("deleting secret %q provider ids: %v", d.URI.ID, toDelete)
		for _, rev := range toDelete {
			if err := secretsBackend.DeleteContent(d.URI, rev); err != nil {
				if errors.Is(err, errors.NotFound) {
					continue
				}
				return errors.Annotatef(err, "cannot delete secret %q revision %d from backend: %v", d.URI.ID, rev, err)
			}
		}
	}

	for _, grants := range ctx.secretChanges.pendingGrants {
		for _, g := range grants {
			pendingGrants = append(pendingGrants, g)
		}
	}

	for _, revokes := range ctx.secretChanges.pendingRevokes {
		pendingRevokes = append(pendingRevokes, revokes...)
	}

	for uri := range ctx.secretChanges.pendingTrackLatest {
		pendingTrackLatest = append(pendingTrackLatest, uri)
	}

	b.AddSecretCreates(pendingCreates)
	b.AddSecretUpdates(pendingUpdates)
	b.AddSecretDeletes(pendingDeletes)
	b.AddSecretGrants(pendingGrants)
	b.AddSecretRevokes(pendingRevokes)
	b.AddTrackLatest(pendingTrackLatest)

	if ctx.modelType == model.CAAS {
		if err := ctx.addCommitHookChangesForCAAS(b, process); err != nil {
			return err
		}
	}

	// Generate change request but skip its execution if no changes are pending.
	commitReq, numChanges := b.Build()
	if numChanges > 0 {
		if err := ctx.unit.CommitHookChanges(commitReq); err != nil {
			ctx.logger.Errorf("cannot apply changes: %v", err)
		cleanupDone:
			for _, secretId := range cleanups {
				if err2 := secretsBackend.DeleteExternalContent(secretId); err2 != nil {
					if errors.Is(err, errors.NotSupported) {
						break cleanupDone
					}
					ctx.logger.Errorf("cannot cleanup secret %q: %v", secretId, err2)
				}
			}
			return errors.Trace(err)
		}
	}

	// Call completed successfully; update local state
	ctx.charmStateCacheDirty = false
	return nil
}

// If we're running the upgrade-charm hook and no podspec update was done,
// we'll still trigger a change to a counter on the podspec so that we can
// ensure any other charm changes (eg storage) are acted on.
func (ctx *HookContext) addCommitHookChangesForCAAS(builder *uniter.CommitHookParamsBuilder, process string) error {
	if ctx.podSpecYaml == nil && ctx.k8sRawSpecYaml == nil && process != string(hooks.UpgradeCharm) {
		// No ops for any situation unless any k8s spec needs to be set or "upgrade-charm" was run.
		return nil
	}
	if ctx.podSpecYaml != nil && ctx.k8sRawSpecYaml != nil {
		return errors.NewForbidden(nil, "either k8s-spec-set or k8s-raw-set can be run for each application, but not both")
	}

	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	// Only leader can set k8s spec.
	if !isLeader {
		if process == string(hooks.UpgradeCharm) {
			// We do not want to fail the non leader unit's upgrade-charm hook.
			return nil
		}
		ctx.logger.Errorf("%v is not the leader but is setting application k8s spec", ctx.unitName)
		return ErrIsNotLeader
	}

	appTag := names.NewApplicationTag(ctx.unit.ApplicationName())
	if ctx.k8sRawSpecYaml != nil {
		builder.SetRawK8sSpec(appTag, ctx.k8sRawSpecYaml)
	} else {
		// either set k8s spec or increment upgrade-counter.
		builder.SetPodSpec(appTag, ctx.podSpecYaml)
	}
	return nil
}

// finalizeAction passes back the final status of an Action hook to state.
// It wraps any errors which occurred in normal behavior of the Action run;
// only errors passed in unhandledErr will be returned.
func (ctx *HookContext) finalizeAction(err, flushErr error) error {
	// TODO (binary132): synchronize with gsamfira's reboot logic
	ctx.actionDataMu.Lock()
	defer ctx.actionDataMu.Unlock()
	message := ctx.actionData.ResultsMessage
	results := ctx.actionData.ResultsMap
	tag := ctx.actionData.Tag
	actionStatus := params.ActionCompleted
	if ctx.actionData.Failed {
		select {
		case <-ctx.actionData.Cancel:
			actionStatus = params.ActionAborted
		default:
			actionStatus = params.ActionFailed
		}
	}

	// If we had an action error, we'll simply encapsulate it in the response
	// and discard the error state.  Actions should not error the uniter.
	if err != nil {
		message = err.Error()
		if charmrunner.IsMissingHookError(err) {
			message = fmt.Sprintf("action not implemented on unit %q", ctx.unitName)
		}
		select {
		case <-ctx.actionData.Cancel:
			actionStatus = params.ActionAborted
		default:
			actionStatus = params.ActionFailed
		}
	}
	if flushErr != nil {
		if results == nil {
			results = map[string]interface{}{}
		}
		if stderr, ok := results["stderr"].(string); ok {
			results["stderr"] = stderr + "\n" + flushErr.Error()
		} else {
			results["stderr"] = flushErr.Error()
		}
		if code, ok := results["return-code"]; !ok || code == "0" {
			results["return-code"] = "1"
		}
		actionStatus = params.ActionFailed
		if message == "" {
			message = "committing requested changes failed"
		}
	}

	callErr := ctx.state.ActionFinish(tag, actionStatus, results, message)
	// Prevent the unit agent from looping if it's impossible to finalise the action.
	if params.IsCodeNotFoundOrCodeUnauthorized(callErr) || params.IsCodeAlreadyExists(callErr) {
		ctx.logger.Warningf("error finalising action %v: %v", tag.Id(), callErr)
		callErr = nil
	}
	return errors.Trace(callErr)
}

// killCharmHook tries to kill the current running charm hook.
func (ctx *HookContext) killCharmHook() error {
	proc := ctx.GetProcess()
	if proc == nil {
		// nothing to kill
		return charmrunner.ErrNoProcess
	}
	ctx.logger.Infof("trying to kill context process %v", proc.Pid())

	tick := ctx.clock.After(0)
	timeout := ctx.clock.After(30 * time.Second)
	for {
		// We repeatedly try to kill the process until we fail; this is
		// because we don't control the *Process, and our clients expect
		// to be able to Wait(); so we can't Wait. We could do better,
		//   but not with a single implementation across all platforms.
		// TODO(gsamfira): come up with a better cross-platform approach.
		select {
		case <-tick:
			err := proc.Kill()
			if err != nil {
				ctx.logger.Infof("kill returned: %s", err)
				ctx.logger.Infof("assuming already killed")
				return nil
			}
		case <-timeout:
			return errors.Errorf("failed to kill context process %v", proc.Pid())
		}
		ctx.logger.Infof("waiting for context process %v to die", proc.Pid())
		tick = ctx.clock.After(100 * time.Millisecond)
	}
}

// UnitWorkloadVersion returns the version of the workload reported by
// the current unit.
// Implements jujuc.HookContext.ContextVersion, part of runner.Context.
func (ctx *HookContext) UnitWorkloadVersion() (string, error) {
	return ctx.state.UnitWorkloadVersion(ctx.unit.Tag())
}

// SetUnitWorkloadVersion sets the current unit's workload version to
// the specified value.
// Implements jujuc.HookContext.ContextVersion, part of runner.Context.
func (ctx *HookContext) SetUnitWorkloadVersion(version string) error {
	return ctx.state.SetUnitWorkloadVersion(ctx.unit.Tag(), version)
}

// NetworkInfo returns the network info for the given bindings on the given relation.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) NetworkInfo(bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error) {
	var relId *int
	if relationId != -1 {
		relId = &relationId
	}
	return ctx.unit.NetworkInfo(bindingNames, relId)
}

// WorkloadName returns the name of the container/workload for workload hooks.
func (ctx *HookContext) WorkloadName() (string, error) {
	if ctx.workloadName == "" {
		return "", errors.NotFoundf("workload name")
	}
	return ctx.workloadName, nil
}

// WorkloadNoticeType returns the type of the notice for workload notice hooks.
func (ctx *HookContext) WorkloadNoticeType() (string, error) {
	if ctx.noticeType == "" {
		return "", errors.NotFoundf("workload notice type")
	}
	return ctx.noticeType, nil
}

// WorkloadNoticeKey returns the key of the notice for workload notice hooks.
func (ctx *HookContext) WorkloadNoticeKey() (string, error) {
	if ctx.noticeKey == "" {
		return "", errors.NotFoundf("workload notice key")
	}
	return ctx.noticeKey, nil
}

// WorkloadCheckName returns the name of the check for workload check hooks.
func (ctx *HookContext) WorkloadCheckName() (string, error) {
	if ctx.checkName == "" {
		return "", errors.NotFoundf("workload check name")
	}
	return ctx.checkName, nil
}

// SecretURI returns the secret URI for secret hooks.
// This is not yet used by any hook commands - it is exported
// for tests to use.
func (ctx *HookContext) SecretURI() (string, error) {
	if ctx.secretURI == "" {
		return "", errors.NotFoundf("secret URI")
	}
	return ctx.secretURI, nil
}

// SecretLabel returns the secret label for secret hooks.
// This is not yet used by any hook commands - it is exported
// for tests to use.
func (ctx *HookContext) SecretLabel() string {
	return ctx.secretLabel
}

// SecretRevision returns the secret revision for secret hooks.
// This is not yet used by any hook commands - it is exported
// for tests to use.
func (ctx *HookContext) SecretRevision() int {
	return ctx.secretRevision
}
