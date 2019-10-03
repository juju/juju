// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package context contains the ContextFactory and Context definitions. Context implements
// hooks.Context and is used together with uniter.Runner to run hooks, commands and actions.
package context

import (
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/proxy"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	k8sspecs "github.com/juju/juju/caas/kubernetes/provider/specs"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
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
var mutex = sync.Mutex{}
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

// HookContext is the implementation of hooks.Context.
type HookContext struct {
	unit *uniter.Unit

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

	// actionData contains the values relevant to the run of an Action:
	// its tag, its parameters, and its results.
	actionData *ActionData

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

	// relations contains the context for every relation the unit is a member
	// of, keyed on relation id.
	relations map[int]*ContextRelation

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
}

// Component implements hooks.Context.
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

func (ctx *HookContext) RequestReboot(priority jujuc.RebootPriority) error {
	// Must set reboot priority first, because killing the hook
	// process will trigger the completion of the hook. If killing
	// the hook fails, then we can reset the priority.
	ctx.SetRebootPriority(priority)

	var err error
	if priority == jujuc.RebootNow {
		// At this point, the hook should be running
		err = ctx.killCharmHook()
	}

	switch err {
	case nil, charmrunner.ErrNoProcess:
		// ErrNoProcess almost certainly means we are running in debug hooks
	default:
		ctx.SetRebootPriority(jujuc.RebootSkip)
	}
	return err
}

func (ctx *HookContext) GetRebootPriority() jujuc.RebootPriority {
	mutex.Lock()
	defer mutex.Unlock()
	return ctx.rebootPriority
}

func (ctx *HookContext) SetRebootPriority(priority jujuc.RebootPriority) {
	mutex.Lock()
	defer mutex.Unlock()
	ctx.rebootPriority = priority
}

func (ctx *HookContext) GetProcess() HookProcess {
	mutex.Lock()
	defer mutex.Unlock()
	return ctx.process
}

func (ctx *HookContext) SetProcess(process HookProcess) {
	mutex.Lock()
	defer mutex.Unlock()
	ctx.process = process
}

func (ctx *HookContext) Id() string {
	return ctx.id
}

func (ctx *HookContext) UnitName() string {
	return ctx.unitName
}

// ModelType of the context we are running in.
func (ctx *HookContext) ModelType() model.ModelType {
	return ctx.modelType
}

// UnitStatus will return the status for the current Unit.
func (ctx *HookContext) UnitStatus() (*jujuc.StatusInfo, error) {
	if ctx.status == nil {
		var err error
		status, err := ctx.unit.UnitStatus()
		if err != nil {
			return nil, err
		}
		ctx.status = &jujuc.StatusInfo{
			Status: status.Status,
			Info:   status.Info,
			Data:   status.Data,
		}
	}
	return ctx.status, nil
}

// ApplicationStatus returns the status for the application and all the units on
// the application to which this context unit belongs, only if this unit is
// the leader.
func (ctx *HookContext) ApplicationStatus() (jujuc.ApplicationStatusInfo, error) {
	var err error
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		return jujuc.ApplicationStatusInfo{}, ErrIsNotLeader
	}
	application, err := ctx.unit.Application()
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}
	status, err := application.Status(ctx.unit.Name())
	if err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}
	us := make([]jujuc.StatusInfo, len(status.Units))
	i := 0
	for t, s := range status.Units {
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
			Tag:    application.Tag().String(),
			Status: status.Application.Status,
			Info:   status.Application.Info,
			Data:   status.Application.Data,
		},
		Units: us,
	}, nil
}

// SetUnitStatus will set the given status for this unit.
func (ctx *HookContext) SetUnitStatus(unitStatus jujuc.StatusInfo) error {
	ctx.hasRunStatusSet = true
	logger.Tracef("[WORKLOAD-STATUS] %s: %s", unitStatus.Status, unitStatus.Info)
	return ctx.unit.SetUnitStatus(
		status.Status(unitStatus.Status),
		unitStatus.Info,
		unitStatus.Data,
	)
}

// SetApplicationStatus will set the given status to the application to which this
// unit's belong, only if this unit is the leader.
func (ctx *HookContext) SetApplicationStatus(applicationStatus jujuc.StatusInfo) error {
	logger.Tracef("[APPLICATION-STATUS] %s: %s", applicationStatus.Status, applicationStatus.Info)
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		return ErrIsNotLeader
	}

	application, err := ctx.unit.Application()
	if err != nil {
		return errors.Trace(err)
	}
	return application.SetStatus(
		ctx.unit.Name(),
		status.Status(applicationStatus.Status),
		applicationStatus.Info,
		applicationStatus.Data,
	)
}

func (ctx *HookContext) HasExecutionSetUnitStatus() bool {
	return ctx.hasRunStatusSet
}

func (ctx *HookContext) ResetExecutionSetUnitStatus() {
	ctx.hasRunStatusSet = false
}

func (ctx *HookContext) PublicAddress() (string, error) {
	if ctx.publicAddress == "" {
		return "", errors.NotFoundf("public address")
	}
	return ctx.publicAddress, nil
}

func (ctx *HookContext) PrivateAddress() (string, error) {
	if ctx.privateAddress == "" {
		return "", errors.NotFoundf("private address")
	}
	return ctx.privateAddress, nil
}

func (ctx *HookContext) AvailabilityZone() (string, error) {
	if ctx.availabilityzone == "" {
		return "", errors.NotFoundf("availability zone")
	}
	return ctx.availabilityzone, nil
}

func (ctx *HookContext) StorageTags() ([]names.StorageTag, error) {
	return ctx.storage.StorageTags()
}

func (ctx *HookContext) HookStorage() (jujuc.ContextStorageAttachment, error) {
	return ctx.Storage(ctx.storageTag)
}

func (ctx *HookContext) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, error) {
	return ctx.storage.Storage(tag)
}

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

func (ctx *HookContext) OpenPorts(protocol string, fromPort, toPort int) error {
	return tryOpenPorts(
		protocol, fromPort, toPort,
		ctx.unit.Tag(),
		ctx.machinePorts, ctx.pendingPorts,
	)
}

func (ctx *HookContext) ClosePorts(protocol string, fromPort, toPort int) error {
	return tryClosePorts(
		protocol, fromPort, toPort,
		ctx.unit.Tag(),
		ctx.machinePorts, ctx.pendingPorts,
	)
}

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

func (ctx *HookContext) GoalState() (*application.GoalState, error) {
	var err error
	ctx.goalState, err = ctx.state.GoalState()
	if err != nil {
		return nil, err
	}

	return &ctx.goalState, nil
}

func (ctx *HookContext) SetPodSpec(specYaml string) error {
	entityName := ctx.unitName
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		logger.Errorf("%v is not the leader but is setting application pod spec", entityName)
		return ErrIsNotLeader
	}
	_, err = k8sspecs.ParsePodSpec(specYaml)
	if err != nil {
		return errors.Trace(err)
	}
	ctx.podSpecYaml = &specYaml
	return nil
}

// CloudSpec return the cloud specification for the running unit's model
func (ctx *HookContext) CloudSpec() (*params.CloudSpec, error) {
	var err error
	ctx.cloudSpec, err = ctx.state.CloudSpec()
	if err != nil {
		return nil, err
	}
	return ctx.cloudSpec, nil
}

// ActionName returns the name of the action.
func (ctx *HookContext) ActionName() (string, error) {
	if ctx.actionData == nil {
		return "", errors.New("not running an action")
	}
	return ctx.actionData.Name, nil
}

// ActionParams simply returns the arguments to the Action.
func (ctx *HookContext) ActionParams() (map[string]interface{}, error) {
	if ctx.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return ctx.actionData.Params, nil
}

// LogActionMessage logs a progress message for the Action.
func (ctx *HookContext) LogActionMessage(message string) error {
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	return ctx.unit.LogActionMessage(ctx.actionData.Tag, message)
}

// SetActionMessage sets a message for the Action, usually an error message.
func (ctx *HookContext) SetActionMessage(message string) error {
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	ctx.actionData.ResultsMessage = message
	return nil
}

// SetActionFailed sets the fail state of the action.
func (ctx *HookContext) SetActionFailed() error {
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
func (ctx *HookContext) UpdateActionResults(keys []string, value string) error {
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	addValueToMap(keys, value, ctx.actionData.ResultsMap)
	return nil
}

func (ctx *HookContext) HookRelation() (jujuc.ContextRelation, error) {
	return ctx.Relation(ctx.relationId)
}

func (ctx *HookContext) RemoteUnitName() (string, error) {
	if ctx.remoteUnitName == "" {
		return "", errors.NotFoundf("remote unit")
	}
	return ctx.remoteUnitName, nil
}

func (ctx *HookContext) Relation(id int) (jujuc.ContextRelation, error) {
	r, found := ctx.relations[id]
	if !found {
		return nil, errors.NotFoundf("relation")
	}
	return r, nil
}

func (ctx *HookContext) RelationIds() ([]int, error) {
	ids := []int{}
	for id := range ctx.relations {
		ids = append(ids, id)
	}
	return ids, nil
}

// AddMetric adds metrics to the hook context.
func (ctx *HookContext) AddMetric(key, value string, created time.Time) error {
	return errors.New("metrics not allowed in this context")
}

// AddMetricLabels adds metrics with labels to the hook context.
func (ctx *HookContext) AddMetricLabels(key, value string, created time.Time, labels map[string]string) error {
	return errors.New("metrics not allowed in this context")
}

// ActionData returns the context's internal action data. It's meant to be
// transitory; it exists to allow uniter and runner code to keep working as
// it did; it should be considered deprecated, and not used by new clients.
func (c *HookContext) ActionData() (*ActionData, error) {
	if c.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return c.actionData, nil
}

// HookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into context.
func (context *HookContext) HookVars(paths Paths, remote bool) ([]string, error) {
	vars := context.legacyProxySettings.AsEnvironmentValues()
	// TODO(thumper): as work on proxies progress, there will be additional
	// proxy settings to be added.
	vars = append(vars,
		"CHARM_DIR="+paths.GetCharmDir(), // legacy, embarrassing
		"JUJU_CHARM_DIR="+paths.GetCharmDir(),
		"JUJU_CONTEXT_ID="+context.id,
		"JUJU_AGENT_SOCKET_ADDRESS="+paths.GetJujucClientSocket(remote).Address,
		"JUJU_AGENT_SOCKET_NETWORK="+paths.GetJujucClientSocket(remote).Network,
		"JUJU_UNIT_NAME="+context.unitName,
		"JUJU_MODEL_UUID="+context.uuid,
		"JUJU_MODEL_NAME="+context.modelName,
		"JUJU_API_ADDRESSES="+strings.Join(context.apiAddrs, " "),
		"JUJU_SLA="+context.slaLevel,
		"JUJU_MACHINE_ID="+context.assignedMachineTag.Id(),
		"JUJU_PRINCIPAL_UNIT="+context.principal,
		"JUJU_AVAILABILITY_ZONE="+context.availabilityzone,
		"JUJU_VERSION="+version.Current.String(),
		"CLOUD_API_VERSION="+context.cloudAPIVersion,
		// Some of these will be empty, but that is fine, better
		// to explicitly export them as empty.
		"JUJU_CHARM_HTTP_PROXY="+context.jujuProxySettings.Http,
		"JUJU_CHARM_HTTPS_PROXY="+context.jujuProxySettings.Https,
		"JUJU_CHARM_FTP_PROXY="+context.jujuProxySettings.Ftp,
		"JUJU_CHARM_NO_PROXY="+context.jujuProxySettings.NoProxy,
	)
	if remote {
		vars = append(vars,
			"JUJU_AGENT_CA_CERT="+path.Join(paths.GetBaseDir(), caas.CACertFile),
		)
	}
	if context.meterStatus != nil {
		vars = append(vars,
			"JUJU_METER_STATUS="+context.meterStatus.code,
			"JUJU_METER_INFO="+context.meterStatus.info,
		)

	}
	if r, err := context.HookRelation(); err == nil {
		vars = append(vars,
			"JUJU_RELATION="+r.Name(),
			"JUJU_RELATION_ID="+r.FakeId(),
			"JUJU_REMOTE_UNIT="+context.remoteUnitName,
		)
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if context.actionData != nil {
		vars = append(vars,
			"JUJU_ACTION_NAME="+context.actionData.Name,
			"JUJU_ACTION_UUID="+context.actionData.Tag.Id(),
			"JUJU_ACTION_TAG="+context.actionData.Tag.String(),
		)
	}

	return append(vars, OSDependentEnvVars(paths)...), nil
}

func (ctx *HookContext) handleReboot(err *error) {
	logger.Tracef("checking for reboot request")
	rebootPriority := ctx.GetRebootPriority()
	switch rebootPriority {
	case jujuc.RebootSkip:
		return
	case jujuc.RebootAfterHook:
		// Reboot should happen only after hook has finished.
		if *err != nil {
			return
		}
		*err = ErrReboot
	case jujuc.RebootNow:
		*err = ErrRequeueAndReboot
	}
	err2 := ctx.unit.SetUnitStatus(status.Rebooting, "", nil)
	if err2 != nil {
		logger.Errorf("updating agent status: %v", err2)
	}
	reqErr := ctx.unit.RequestReboot()
	if reqErr != nil {
		*err = reqErr
	}
}

// Prepare implements the Context interface.
func (ctx *HookContext) Prepare() error {
	if ctx.actionData != nil {
		err := ctx.state.ActionBegin(ctx.actionData.Tag)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Flush implements the Context interface.
func (ctx *HookContext) Flush(process string, ctxErr error) (err error) {
	writeChanges := ctxErr == nil

	// In the case of Actions, handle any errors using finalizeAction.
	if ctx.actionData != nil {
		// If we had an error in err at this point, it's part of the
		// normal behavior of an Action.  Errors which happen during
		// the finalize should be handed back to the uniter.  Close
		// over the existing err, clear it, and only return errors
		// which occur during the finalize, e.g. API call errors.
		defer func(ctxErr error) {
			err = ctx.finalizeAction(ctxErr, err)
		}(ctxErr)
		ctxErr = nil
	} else {
		// TODO(gsamfira): Just for now, reboot will not be supported in actions.
		defer ctx.handleReboot(&err)
	}

	for id, rctx := range ctx.relations {
		if writeChanges {
			if e := rctx.WriteSettings(); e != nil {
				e = errors.Errorf(
					"could not write settings from %q to relation %d: %v",
					process, id, e,
				)
				logger.Errorf("%v", e)
				if ctxErr == nil {
					ctxErr = e
				}
			}
		}
	}

	for rangeKey, rangeInfo := range ctx.pendingPorts {
		if writeChanges {
			var e error
			var op string
			if rangeInfo.ShouldOpen {
				e = ctx.unit.OpenPorts(
					rangeKey.Ports.Protocol,
					rangeKey.Ports.FromPort,
					rangeKey.Ports.ToPort,
				)
				op = "open"
			} else {
				e = ctx.unit.ClosePorts(
					rangeKey.Ports.Protocol,
					rangeKey.Ports.FromPort,
					rangeKey.Ports.ToPort,
				)
				op = "close"
			}
			if e != nil {
				e = errors.Annotatef(e, "cannot %s %v", op, rangeKey.Ports)
				logger.Errorf("%v", e)
				if ctxErr == nil {
					ctxErr = e
				}
			}
		}
	}

	// add storage to unit dynamically
	if len(ctx.storageAddConstraints) > 0 && writeChanges {
		err := ctx.unit.AddStorage(ctx.storageAddConstraints)
		if err != nil {
			err = errors.Annotatef(err, "cannot add storage")
			logger.Errorf("%v", err)
			if ctxErr == nil {
				ctxErr = err
			}
		}
	}

	if ctx.podSpecYaml != nil && writeChanges {
		err := ctx.commitPodSpec()
		if ctxErr == nil {
			ctxErr = err
		}
	}

	// TODO (tasdomas) 2014 09 03: context finalization needs to modified to apply all
	//                             changes in one api call to minimize the risk
	//                             of partial failures.

	if !writeChanges {
		return ctxErr
	}

	return ctxErr
}

// commitPodSpec dispatches pending SetPodSpec call.
func (ctx *HookContext) commitPodSpec() error {
	if ctx.podSpecYaml == nil {
		return nil
	}
	specYaml := *ctx.podSpecYaml
	entityName := ctx.unitName
	isLeader, err := ctx.IsLeader()
	if err != nil {
		return errors.Annotatef(err, "cannot determine leadership")
	}
	if !isLeader {
		logger.Errorf("%v is not the leader but is setting application pod spec", entityName)
		return ErrIsNotLeader
	}
	entityName = ctx.unit.ApplicationName()
	err = ctx.state.SetPodSpec(entityName, specYaml)
	if err != nil {
		if err2 := ctx.SetApplicationStatus(jujuc.StatusInfo{
			Status: status.Blocked.String(),
			Info:   fmt.Sprintf("setting pod spec: %v", err),
		}); err2 != nil {
			logger.Errorf("updating agent status: %v", err2)
		}
	}
	return nil
}

// finalizeAction passes back the final status of an Action hook to state.
// It wraps any errors which occurred in normal behavior of the Action run;
// only errors passed in unhandledErr will be returned.
func (ctx *HookContext) finalizeAction(err, unhandledErr error) error {
	// TODO (binary132): synchronize with gsamfira's reboot logic
	message := ctx.actionData.ResultsMessage
	results := ctx.actionData.ResultsMap
	tag := ctx.actionData.Tag
	status := params.ActionCompleted
	if ctx.actionData.Failed {
		status = params.ActionFailed
	}

	// If we had an action error, we'll simply encapsulate it in the response
	// and discard the error state.  Actions should not error the uniter.
	if err != nil {
		message = err.Error()
		if charmrunner.IsMissingHookError(err) {
			message = fmt.Sprintf("action not implemented on unit %q", ctx.unitName)
		}
		status = params.ActionFailed
	}

	callErr := ctx.state.ActionFinish(tag, status, results, message)
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
func (ctx *HookContext) NetworkInfo(bindingNames []string, relationId int) (map[string]params.NetworkInfoResult, error) {
	var relId *int
	if relationId != -1 {
		relId = &relationId
	}
	return ctx.unit.NetworkInfo(bindingNames, relId)
}
