// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/proxy"
	"gopkg.in/juju/charm.v5-unstable"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.worker.uniter.context")
var mutex = sync.Mutex{}

// meterStatus describes the unit's meter status.
type meterStatus struct {
	code string
	info string
}

// HookContext is the implementation of jujuc.Context.
type HookContext struct {
	unit *uniter.Unit

	// state is the handle to the uniter State so that HookContext can make
	// API calls on the stateservice.
	// NOTE: We would like to be rid of the fake-remote-Unit and switch
	// over fully to API calls on State.  This adds that ability, but we're
	// not fully there yet.
	state *uniter.State

	// LeadershipContext supplies several jujuc.Context methods.
	LeadershipContext

	// privateAddress is the cached value of the unit's private
	// address.
	privateAddress string

	// publicAddress is the cached value of the unit's public
	// address.
	publicAddress string

	// availabilityzone is the cached value of the unit's availability zone name.
	availabilityzone string

	// configSettings holds the service configuration.
	configSettings charm.Settings

	// id identifies the context.
	id string

	// actionData contains the values relevant to the run of an Action:
	// its tag, its parameters, and its results.
	actionData *ActionData

	// uuid is the universally unique identifier of the environment.
	uuid string

	// envName is the human friendly name of the environment.
	envName string

	// unitName is the human friendly name of the local unit.
	unitName string

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

	// serviceOwner contains the user tag of the service owner.
	serviceOwner names.UserTag

	// proxySettings are the current proxy settings that the uniter knows about.
	proxySettings proxy.Settings

	// metrics are the metrics recorded by calls to add-metric.
	metrics []jujuc.Metric

	// canAddMetrics specifies whether the hook allows recording metrics.
	canAddMetrics bool

	// definedMetrics specifies the metrics the charm has defined in its metrics.yaml file.
	definedMetrics *charm.Metrics

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
	process *os.Process

	// rebootPriority tells us when the hook wants to reboot. If rebootPriority is jujuc.RebootNow
	// the hook will be killed and requeued
	rebootPriority jujuc.RebootPriority

	// storage provides access to the information about storage attached to the unit.
	storage StorageContextAccessor

	// storageId is the tag of the storage instance associated with the running hook.
	storageTag names.StorageTag
}

func (ctx *HookContext) RequestReboot(priority jujuc.RebootPriority) error {
	var err error
	if priority == jujuc.RebootNow {
		// At this point, the hook should be running
		err = ctx.killCharmHook()
	}

	switch err {
	case nil, ErrNoProcess:
		// ErrNoProcess almost certainly means we are running in debug hooks
		ctx.SetRebootPriority(priority)
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

func (ctx *HookContext) GetProcess() *os.Process {
	mutex.Lock()
	defer mutex.Unlock()
	return ctx.process
}

func (ctx *HookContext) SetProcess(process *os.Process) {
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

func (ctx *HookContext) PublicAddress() (string, bool) {
	return ctx.publicAddress, ctx.publicAddress != ""
}

func (ctx *HookContext) PrivateAddress() (string, bool) {
	return ctx.privateAddress, ctx.privateAddress != ""
}

func (ctx *HookContext) AvailabilityZone() (string, bool) {
	return ctx.availabilityzone, ctx.availabilityzone != ""
}

func (ctx *HookContext) HookStorage() (jujuc.ContextStorage, bool) {
	return ctx.Storage(ctx.storageTag)
}

func (ctx *HookContext) Storage(tag names.StorageTag) (jujuc.ContextStorage, bool) {
	return ctx.storage.Storage(tag)
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

func (ctx *HookContext) OwnerTag() string {
	return ctx.serviceOwner.String()
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

// ActionName returns the name of the action.
func (ctx *HookContext) ActionName() (string, error) {
	if ctx.actionData == nil {
		return "", errors.New("not running an action")
	}
	return ctx.actionData.ActionName, nil
}

// ActionParams simply returns the arguments to the Action.
func (ctx *HookContext) ActionParams() (map[string]interface{}, error) {
	if ctx.actionData == nil {
		return nil, errors.New("not running an action")
	}
	return ctx.actionData.ActionParams, nil
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
	ctx.actionData.ActionFailed = true
	return nil
}

// UpdateActionResults inserts new values for use with action-set and
// action-fail.  The results struct will be delivered to the state server
// upon completion of the Action.  It returns an error if not called on an
// Action-containing HookContext.
func (ctx *HookContext) UpdateActionResults(keys []string, value string) error {
	if ctx.actionData == nil {
		return errors.New("not running an action")
	}
	addValueToMap(keys, value, ctx.actionData.ResultsMap)
	return nil
}

func (ctx *HookContext) HookRelation() (jujuc.ContextRelation, bool) {
	return ctx.Relation(ctx.relationId)
}

func (ctx *HookContext) RemoteUnitName() (string, bool) {
	return ctx.remoteUnitName, ctx.remoteUnitName != ""
}

func (ctx *HookContext) Relation(id int) (jujuc.ContextRelation, bool) {
	r, found := ctx.relations[id]
	return r, found
}

func (ctx *HookContext) RelationIds() []int {
	ids := []int{}
	for id := range ctx.relations {
		ids = append(ids, id)
	}
	return ids
}

// AddMetrics adds metrics to the hook context.
func (ctx *HookContext) AddMetric(key, value string, created time.Time) error {
	if !ctx.canAddMetrics || ctx.definedMetrics == nil {
		return errors.New("metrics disabled")
	}
	err := ctx.definedMetrics.ValidateMetric(key, value)
	if err != nil {
		return errors.Annotatef(err, "invalid metric %q", key)
	}
	ctx.metrics = append(ctx.metrics, jujuc.Metric{key, value, created})
	return nil
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
func (context *HookContext) HookVars(paths Paths) []string {
	vars := context.proxySettings.AsEnvironmentValues()
	vars = append(vars,
		"CHARM_DIR="+paths.GetCharmDir(), // legacy, embarrassing
		"JUJU_CHARM_DIR="+paths.GetCharmDir(),
		"JUJU_CONTEXT_ID="+context.id,
		"JUJU_AGENT_SOCKET="+paths.GetJujucSocket(),
		"JUJU_UNIT_NAME="+context.unitName,
		"JUJU_ENV_UUID="+context.uuid,
		"JUJU_ENV_NAME="+context.envName,
		"JUJU_API_ADDRESSES="+strings.Join(context.apiAddrs, " "),
		"JUJU_METER_STATUS="+context.meterStatus.code,
		"JUJU_METER_INFO="+context.meterStatus.info,
		"JUJU_MACHINE_ID="+context.assignedMachineTag.Id(),
		"JUJU_AVAILABILITY_ZONE="+context.availabilityzone,
	)
	if r, found := context.HookRelation(); found {
		vars = append(vars,
			"JUJU_RELATION="+r.Name(),
			"JUJU_RELATION_ID="+r.FakeId(),
			"JUJU_REMOTE_UNIT="+context.remoteUnitName,
		)
	}
	if context.actionData != nil {
		vars = append(vars,
			"JUJU_ACTION_NAME="+context.actionData.ActionName,
			"JUJU_ACTION_UUID="+context.actionData.ActionTag.Id(),
			"JUJU_ACTION_TAG="+context.actionData.ActionTag.String(),
		)
	}
	return append(vars, osDependentEnvVars(paths)...)
}

func (ctx *HookContext) handleReboot(err *error) {
	logger.Infof("handling reboot")
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
	reqErr := ctx.unit.RequestReboot()
	if reqErr != nil {
		*err = reqErr
	}
}

func (ctx *HookContext) FlushContext(process string, ctxErr error) (err error) {
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
	if ctxErr != nil {
		return ctxErr
	}

	// TODO (tasdomas) 2014 09 03: context finalization needs to modified to apply all
	//                             changes in one api call to minimize the risk
	//                             of partial failures.
	if ctx.canAddMetrics && len(ctx.metrics) > 0 {
		if writeChanges {
			metrics := make([]params.Metric, len(ctx.metrics))
			for i, metric := range ctx.metrics {
				metrics[i] = params.Metric{Key: metric.Key, Value: metric.Value, Time: metric.Time}
			}
			if e := ctx.unit.AddMetrics(metrics); e != nil {
				logger.Errorf("%v", e)
				if ctxErr == nil {
					ctxErr = e
				}
			}
		}
		ctx.metrics = nil
	}

	return ctxErr
}

// finalizeAction passes back the final status of an Action hook to state.
// It wraps any errors which occurred in normal behavior of the Action run;
// only errors passed in unhandledErr will be returned.
func (ctx *HookContext) finalizeAction(err, unhandledErr error) error {
	// TODO (binary132): synchronize with gsamfira's reboot logic
	message := ctx.actionData.ResultsMessage
	results := ctx.actionData.ResultsMap
	tag := ctx.actionData.ActionTag
	status := params.ActionCompleted
	if ctx.actionData.ActionFailed {
		status = params.ActionFailed
	}

	// If we had an action error, we'll simply encapsulate it in the response
	// and discard the error state.  Actions should not error the uniter.
	if err != nil {
		message = err.Error()
		if IsMissingHookError(err) {
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
		return ErrNoProcess
	}
	logger.Infof("trying to kill context process %d", proc.Pid)

	tick := time.After(0)
	timeout := time.After(30 * time.Second)
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
			return errors.Errorf("failed to kill context process %d", proc.Pid)
		}
		logger.Infof("waiting for context process %d to die", proc.Pid)
		tick = time.After(100 * time.Millisecond)
	}
}
