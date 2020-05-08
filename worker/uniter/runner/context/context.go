// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package context contains the ContextFactory and Context definitions. Context implements
// runner.Context and is used together with uniter.Runner to run hooks, commands and actions.
package context

import (
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/quota"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Paths exposes the paths needed by Context.
type Paths interface {
	// GetToolsDir returns the filesystem path to the dirctory containing
	// the hook tool symlinks.
	GetToolsDir() string

	// GetCharmDir returns the filesystem path to the directory in which
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

	// ComponentDir returns the filesystem path to the directory
	// containing all data files for a component.
	ComponentDir(name string) string
}

// Clock defines the methods of the full clock.Clock that are needed here.
type Clock interface {
	// After waits for the duration to elapse and then sends the
	// current time on the returned channel.
	After(time.Duration) <-chan time.Time
}

var logger = loggo.GetLogger("juju.worker.uniter.context")
var ErrIsNotLeader = errors.Errorf("this unit is not the leader")

// ComponentConfig holds all the information related to a hook context
// needed by components.
type ComponentConfig struct {
	// UnitName is the name of the unit.
	UnitName string
	// DataDir is the component's data directory.
	DataDir string
	// APICaller is the API caller the component may use.
	APICaller base.APICaller
}

// ComponentFunc is a factory function for Context components.
type ComponentFunc func(ComponentConfig) (jujuc.ContextComponent, error)

var registeredComponentFuncs = map[string]ComponentFunc{}

// Add the named component factory func to the registry.
func RegisterComponentFunc(name string, f ComponentFunc) error {
	if _, ok := registeredComponentFuncs[name]; ok {
		return errors.AlreadyExistsf("%s", name)
	}
	registeredComponentFuncs[name] = f
	return nil
}

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

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/hookunit_mock.go github.com/juju/juju/worker/uniter/runner/context HookUnit

// HookUnit represents the functions needed by a unit in a hook context to
// call into state.
type HookUnit interface {
	Application() (*uniter.Application, error)
	ApplicationName() string
	ClosePorts(protocol string, fromPort, toPort int) error
	ConfigSettings() (charm.Settings, error)
	LogActionMessage(names.ActionTag, string) error
	Name() string
	NetworkInfo(bindings []string, relationId *int) (map[string]params.NetworkInfoResult, error)
	OpenPorts(protocol string, fromPort, toPort int) error
	RequestReboot() error
	SetUnitStatus(unitStatus status.Status, info string, data map[string]interface{}) error
	SetAgentStatus(agentStatus status.Status, info string, data map[string]interface{}) error
	State() (params.UnitStateResult, error)
	Tag() names.UnitTag
	UnitStatus() (params.StatusResult, error)
	UpdateNetworkInfo() error
	CommitHookChanges(params.CommitHookChangesArgs) error
}

// HookContext is the implementation of runner.Context.
type HookContext struct {
	unit HookUnit

	// state is the handle to the uniter State so that HookContext can make
	// API calls on the state.
	// NOTE: We would like to be rid of the fake-remote-Unit and switch
	// over fully to API calls on State.  This adds that ability, but we're
	// not fully there yet.
	state *uniter.State

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

	// availabilityzone is the cached value of the unit's availability zone name.
	availabilityzone string

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

	// legacyProxySettings are the current legacy proxy settings that the uniter knows about.
	legacyProxySettings proxy.Settings

	// jujuProxySettings are the current juju proxy settings that the uniter knows about.
	jujuProxySettings proxy.Settings

	// meterStatus is the status of the unit's metering.
	meterStatus *meterStatus

	// pendingPorts contains a list of port ranges to be opened or
	// closed when the current hook is committed.
	pendingPorts map[PortRange]PortRangeInfo

	// machinePorts contains cached information about all opened port
	// ranges on the unit's assigned machine, mapped to the unit that
	// opened each range and the relevant relation.
	machinePorts map[network.PortRange]params.RelationUnit

	// assignedMachineTag contains the tag of the unit's assigned
	// machine.
	assignedMachineTag names.MachineTag

	// process is the process of the command that is being run in the local context,
	// like a juju-run command or a hook
	process HookProcess

	// rebootPriority tells us when the hook wants to reboot. If rebootPriority is hooks.RebootNow
	// the hook will be killed and requeued
	rebootPriority jujuc.RebootPriority

	// storage provides access to the information about storage attached to the unit.
	storage StorageContextAccessor

	// storageId is the tag of the storage instance associated with the running hook.
	storageTag names.StorageTag

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

	componentDir   func(string) string
	componentFuncs map[string]ComponentFunc

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

	mu sync.Mutex
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

// Component returns the ContextComponent with the supplied name if
// it was found.
// Implements jujuc.HookContext.ContextComponents, part of runner.Context.
func (ctx *HookContext) Component(name string) (jujuc.ContextComponent, error) {
	compCtxFunc, ok := ctx.componentFuncs[name]
	if !ok {
		return nil, errors.NotFoundf("context component %q", name)
	}

	facade := ctx.state.Facade()
	config := ComponentConfig{
		UnitName:  ctx.unit.Name(),
		DataDir:   ctx.componentDir(name),
		APICaller: facade.RawAPICaller(),
	}
	compCtx, err := compCtxFunc(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return compCtx, nil
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
	logger.Tracef("[WORKLOAD-STATUS] %s: %s", unitStatus.Status, unitStatus.Info)
	return ctx.unit.SetUnitStatus(
		status.Status(unitStatus.Status),
		unitStatus.Info,
		unitStatus.Data,
	)
}

// SetAgentStatus will set the given status for this unit's agent.
// Implements jujuc.HookContext.ContextStatus, part of runner.Context.
func (ctx *HookContext) SetAgentStatus(agentStatus jujuc.StatusInfo) error {
	logger.Tracef("[AGENT-STATUS] %s: %s", agentStatus.Status, agentStatus.Info)
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
	logger.Tracef("[APPLICATION-STATUS] %s: %s", applicationStatus.Status, applicationStatus.Info)
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

// PublicAddress returns the executing unit's public address or an
// error if it is not available.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) PublicAddress() (string, error) {
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
	if ctx.availabilityzone == "" {
		return "", errors.NotFoundf("availability zone")
	}
	return ctx.availabilityzone, nil
}

// StorageTags returns a list of tags for storage instances
// attached to the unit or an error if they are not available.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) StorageTags() ([]names.StorageTag, error) {
	return ctx.storage.StorageTags()
}

// HookStorage returns the storage attachment associated
// the executing hook if it was found, and an error if it
// was not found or is not available.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) HookStorage() (jujuc.ContextStorageAttachment, error) {
	return ctx.Storage(ctx.storageTag)
}

// Storage returns the ContextStorageAttachment with the supplied
// tag if it was found, and an error if it was not found or is not
// available to the context.
// Implements jujuc.HookContext.ContextStorage, part of runner.Context.
func (ctx *HookContext) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, error) {
	return ctx.storage.Storage(tag)
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

// OpenPorts marks the supplied port range for opening when the
// executing unit's application is exposed.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) OpenPorts(protocol string, fromPort, toPort int) error {
	return tryOpenPorts(
		protocol, fromPort, toPort,
		ctx.unit.Tag(),
		ctx.machinePorts, ctx.pendingPorts,
	)
}

// ClosePorts ensures the supplied port range is closed even when
// the executing unit's application is exposed (unless it is opened
// separately by a co- located unit).
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) ClosePorts(protocol string, fromPort, toPort int) error {
	return tryClosePorts(
		protocol, fromPort, toPort,
		ctx.unit.Tag(),
		ctx.machinePorts, ctx.pendingPorts,
	)
}

// OpenedPorts returns all port ranges currently opened by this
// unit on its assigned machine. The result is sorted first by
// protocol, then by number.
// Implements jujuc.HookContext.ContextNetworking, part of runner.Context.
func (ctx *HookContext) OpenedPorts() []network.PortRange {
	var unitRanges []network.PortRange
	for portRange, relUnit := range ctx.machinePorts {
		if relUnit.Unit == ctx.unit.Tag().String() {
			unitRanges = append(unitRanges, portRange)
		}
	}
	network.SortPortRanges(unitRanges)
	return unitRanges
}

// Config returns the current application configuration of the executing unit.
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
		logger.Errorf("%q is not the leader but is setting application k8s spec", ctx.unitName)
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
		logger.Errorf("%q is not the leader but is setting application raw k8s spec", ctx.unitName)
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
func (ctx *HookContext) UpdateActionResults(keys []string, value string) error {
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
		return "", errors.NotFoundf("remote application")
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
	for id := range ctx.relations {
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
func (ctx *HookContext) HookVars(paths Paths, remote bool, getEnv GetEnvFunc) ([]string, error) {
	vars := ctx.legacyProxySettings.AsEnvironmentValues()
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
		"JUJU_AVAILABILITY_ZONE="+ctx.availabilityzone,
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
	return append(vars, OSDependentEnvVars(paths, getEnv)...), nil
}

func (ctx *HookContext) handleReboot(ctxErr error) error {
	logger.Tracef("checking for reboot request")
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
		logger.Errorf("updating agent status: %v", err)
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

	for portRange, info := range ctx.pendingPorts {
		if info.ShouldOpen {
			b.OpenPortRange(portRange.Ports.Protocol, portRange.Ports.FromPort, portRange.Ports.ToPort)
		} else {
			b.ClosePortRange(portRange.Ports.Protocol, portRange.Ports.FromPort, portRange.Ports.ToPort)
		}
	}

	if len(ctx.storageAddConstraints) > 0 {
		b.AddStorage(ctx.storageAddConstraints)
	}

	if ctx.modelType == model.CAAS {
		if err := ctx.addCommitHookChangesForCAAS(b, process); err != nil {
			return err
		}
	}

	// Generate change request but skip its execution if no changes are pending.
	commitReq, numChanges := b.Build()
	if numChanges > 0 {
		if err := ctx.unit.CommitHookChanges(commitReq); err != nil {
			err = errors.Annotatef(err, "cannot apply changes")
			logger.Errorf("%v", err)
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
		logger.Errorf("%v is not the leader but is setting application k8s spec", ctx.unitName)
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
func (ctx *HookContext) finalizeAction(err, unhandledErr error) error {
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

	// If the action completed without an error but we failed to flush the
	// charm state changes due to a quota limit, we should attach the error
	// to the action.
	if err == nil && errors.IsQuotaLimitExceeded(unhandledErr) {
		err = unhandledErr
		unhandledErr = nil
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

	callErr := ctx.state.ActionFinish(tag, actionStatus, results, message)
	if callErr != nil {
		unhandledErr = errors.Wrap(unhandledErr, callErr)
	}
	return unhandledErr
}

// killCharmHook tries to kill the current running charm hook.
func (ctx *HookContext) killCharmHook() error {
	proc := ctx.GetProcess()
	if proc == nil {
		// nothing to kill
		return charmrunner.ErrNoProcess
	}
	logger.Infof("trying to kill context process %v", proc.Pid())

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
				logger.Infof("kill returned: %s", err)
				logger.Infof("assuming already killed")
				return nil
			}
		case <-timeout:
			return errors.Errorf("failed to kill context process %v", proc.Pid())
		}
		logger.Infof("waiting for context process %v to die", proc.Pid())
		tick = ctx.clock.After(100 * time.Millisecond)
	}
}

// UnitWorkloadVersion returns the version of the workload reported by
// the current unit.
// Implements jujuc.HookContext.ContextVersion, part of runner.Context.
func (ctx *HookContext) UnitWorkloadVersion() (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: ctx.unit.Tag().String()}},
	}
	err := ctx.state.Facade().FacadeCall("WorkloadVersion", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}

// SetUnitWorkloadVersion sets the current unit's workload version to
// the specified value.
// Implements jujuc.HookContext.ContextVersion, part of runner.Context.
func (ctx *HookContext) SetUnitWorkloadVersion(version string) error {
	var result params.ErrorResults
	args := params.EntityWorkloadVersions{
		Entities: []params.EntityWorkloadVersion{
			{Tag: ctx.unit.Tag().String(), WorkloadVersion: version},
		},
	}
	err := ctx.state.Facade().FacadeCall("SetWorkloadVersion", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
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
