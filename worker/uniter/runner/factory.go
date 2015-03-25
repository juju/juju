// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5-unstable"
	"gopkg.in/juju/charm.v5-unstable/hooks"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type CommandInfo struct {
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// ForceRemoteUnit skips unit inference and existence validation.
	ForceRemoteUnit bool
}

// Factory represents a long-lived object that can create execution contexts
// relevant to a specific unit. In its current state, it is somewhat bizarre
// and inconsistent; its main value is as an evolutionary step towards a better
// division of responsibilities across worker/uniter and its subpackages.
type Factory interface {

	// NewCommandRunner returns an execution context suitable for running
	// an arbitrary script.
	NewCommandRunner(commandInfo CommandInfo) (Runner, error)

	// NewHookRunner returns an execution context suitable for running the
	// supplied hook definition (which must be valid).
	NewHookRunner(hookInfo hook.Info) (Runner, error)

	// NewActionRunner returns an execution context suitable for running the
	// action identified by the supplied id.
	NewActionRunner(actionId string) (Runner, error)
}

// StorageContextAccessor is an interface providing access to StorageContexts
// for a jujuc.Context.
type StorageContextAccessor interface {

	// Storage returns the jujuc.ContextStorage with the supplied tag if
	// it was found, and whether it was found.
	Storage(names.StorageTag) (jujuc.ContextStorage, bool)
}

// RelationsFunc is used to get snapshots of relation membership at context
// creation time.
type RelationsFunc func() map[int]*RelationInfo

// NewFactory returns a Factory capable of creating execution contexts backed
// by the supplied unit's supplied API connection.
func NewFactory(
	state *uniter.State,
	unitTag names.UnitTag,
	tracker leadership.Tracker,
	getRelationInfos RelationsFunc,
	storage StorageContextAccessor,
	paths Paths,
) (
	Factory, error,
) {
	unit, err := state.Unit(unitTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	service, err := state.Service(unit.ServiceTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ownerTag, err := service.OwnerTag()
	if err != nil {
		return nil, errors.Trace(err)
	}
	machineTag, err := unit.AssignedMachine()
	if err != nil {
		return nil, errors.Trace(err)
	}
	environment, err := state.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &factory{
		unit:             unit,
		state:            state,
		tracker:          tracker,
		paths:            paths,
		envUUID:          environment.UUID(),
		envName:          environment.Name(),
		machineTag:       machineTag,
		ownerTag:         ownerTag,
		getRelationInfos: getRelationInfos,
		relationCaches:   map[int]*RelationCache{},
		storage:          storage,
		rand:             rand.New(rand.NewSource(time.Now().Unix())),
	}, nil
}

type factory struct {
	// API connection fields; unit should be deprecated, but isn't yet.
	unit    *uniter.Unit
	state   *uniter.State
	tracker leadership.Tracker

	// Fields that shouldn't change in a factory's lifetime.
	paths      Paths
	envUUID    string
	envName    string
	machineTag names.MachineTag
	ownerTag   names.UserTag
	storage    StorageContextAccessor

	// Callback to get relation state snapshot.
	getRelationInfos RelationsFunc
	relationCaches   map[int]*RelationCache

	// For generating "unique" context ids.
	rand *rand.Rand
}

// NewCommandRunner exists to satisfy the Factory interface.
func (f *factory) NewCommandRunner(commandInfo CommandInfo) (Runner, error) {
	ctx, err := f.coreContext()
	if err != nil {
		return nil, errors.Trace(err)
	}
	relationId, remoteUnitName, err := inferRemoteUnit(ctx.relations, commandInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx.relationId = relationId
	ctx.remoteUnitName = remoteUnitName
	ctx.id = f.newId("run-commands")
	runner := NewRunner(ctx, f.paths)
	return runner, nil
}

// NewHookRunner exists to satisfy the Factory interface.
func (f *factory) NewHookRunner(hookInfo hook.Info) (Runner, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	ctx, err := f.coreContext()
	if err != nil {
		return nil, errors.Trace(err)
	}

	hookName := string(hookInfo.Kind)
	if hookInfo.Kind.IsRelation() {
		ctx.relationId = hookInfo.RelationId
		ctx.remoteUnitName = hookInfo.RemoteUnit
		relation, found := ctx.relations[hookInfo.RelationId]
		if !found {
			return nil, errors.Errorf("unknown relation id: %v", hookInfo.RelationId)
		}
		if hookInfo.Kind == hooks.RelationDeparted {
			relation.cache.RemoveMember(hookInfo.RemoteUnit)
		} else if hookInfo.RemoteUnit != "" {
			// Clear remote settings cache for changing remote unit.
			relation.cache.InvalidateMember(hookInfo.RemoteUnit)
		}
		hookName = fmt.Sprintf("%s-%s", relation.Name(), hookInfo.Kind)
	}
	if hookInfo.Kind.IsStorage() {
		ctx.storageTag = names.NewStorageTag(hookInfo.StorageId)
		if _, found := ctx.storage.Storage(ctx.storageTag); !found {
			return nil, errors.Errorf("unknown storage id: %v", hookInfo.StorageId)
		}
		storageName, err := names.StorageName(hookInfo.StorageId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		hookName = fmt.Sprintf("%s-%s", storageName, hookName)
	}
	// Metrics are only sent from the collect-metrics hook.
	if hookInfo.Kind == hooks.CollectMetrics {
		ctx.canAddMetrics = true
		ch, err := f.getCharm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		ctx.definedMetrics = ch.Metrics()
	}
	ctx.id = f.newId(hookName)
	runner := NewRunner(ctx, f.paths)
	return runner, nil
}

// NewActionRunner exists to satisfy the Factory interface.
func (f *factory) NewActionRunner(actionId string) (Runner, error) {
	ch, err := f.getCharm()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ok := names.IsValidAction(actionId)
	if !ok {
		return nil, &badActionError{actionId, "not valid actionId"}
	}
	tag := names.NewActionTag(actionId)
	action, err := f.state.Action(tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, ErrActionNotAvailable
	} else if params.IsCodeActionNotAvailable(err) {
		return nil, ErrActionNotAvailable
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	err = f.state.ActionBegin(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	name := action.Name()
	spec, ok := ch.Actions().ActionSpecs[name]
	if !ok {
		return nil, &badActionError{name, "not defined"}
	}
	params := action.Params()
	if err := spec.ValidateParams(params); err != nil {
		return nil, &badActionError{name, err.Error()}
	}

	ctx, err := f.coreContext()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctx.actionData = newActionData(name, &tag, params)
	ctx.id = f.newId(name)
	runner := NewRunner(ctx, f.paths)
	return runner, nil
}

// newId returns a probably-unique identifier for a new context, containing the
// supplied string.
func (f *factory) newId(name string) string {
	return fmt.Sprintf("%s-%s-%d", f.unit.Name(), name, f.rand.Int63())
}

// coreContext creates a new context with all unspecialised fields filled in.
func (f *factory) coreContext() (*HookContext, error) {
	leadershipContext := newLeadershipContext(
		f.state.LeadershipSettings,
		f.tracker,
	)
	ctx := &HookContext{
		unit:               f.unit,
		state:              f.state,
		LeadershipContext:  leadershipContext,
		uuid:               f.envUUID,
		envName:            f.envName,
		unitName:           f.unit.Name(),
		assignedMachineTag: f.machineTag,
		serviceOwner:       f.ownerTag,
		relations:          f.getContextRelations(),
		relationId:         -1,
		canAddMetrics:      false,
		definedMetrics:     nil,
		pendingPorts:       make(map[PortRange]PortRangeInfo),
		storage:            f.storage,
	}
	if err := f.updateContext(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (f *factory) getCharm() (charm.Charm, error) {
	ch, err := charm.ReadCharm(f.paths.GetCharmDir())
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// getContextRelations updates the factory's relation caches, and uses them
// to construct ContextRelations for a fresh context.
func (f *factory) getContextRelations() map[int]*ContextRelation {
	contextRelations := map[int]*ContextRelation{}
	relationInfos := f.getRelationInfos()
	relationCaches := map[int]*RelationCache{}
	for id, info := range relationInfos {
		relationUnit := info.RelationUnit
		memberNames := info.MemberNames
		cache, found := f.relationCaches[id]
		if found {
			cache.Prune(memberNames)
		} else {
			cache = NewRelationCache(relationUnit.ReadSettings, memberNames)
		}
		relationCaches[id] = cache
		contextRelations[id] = NewContextRelation(relationUnit, cache)
	}
	f.relationCaches = relationCaches
	return contextRelations
}

// updateContext fills in all unspecialized fields that require an API call to
// discover.
//
// Approximately *every* line of code in this function represents a bug: ie, some
// piece of information we expose to the charm but which we fail to report changes
// to via hooks. Furthermore, the fact that we make multiple API calls at this
// time, rather than grabbing everything we need in one go, is unforgivably yucky.
func (f *factory) updateContext(ctx *HookContext) (err error) {
	defer errors.Trace(err)

	ctx.apiAddrs, err = f.state.APIAddresses()
	if err != nil {
		return err
	}
	ctx.machinePorts, err = f.state.AllMachinePorts(f.machineTag)
	if err != nil {
		return errors.Trace(err)
	}

	statusCode, statusInfo, err := f.unit.MeterStatus()
	if err != nil {
		return errors.Annotate(err, "could not retrieve meter status for unit")
	}
	ctx.meterStatus = &meterStatus{
		code: statusCode,
		info: statusInfo,
	}

	// TODO(fwereade) 23-10-2014 bug 1384572
	// Nothing here should ever be getting the environ config directly.
	environConfig, err := f.state.EnvironConfig()
	if err != nil {
		return err
	}
	ctx.proxySettings = environConfig.ProxySettings()

	// Calling these last, because there's a potential race: they're not guaranteed
	// to be set in time to be needed for a hook. If they're not, we just leave them
	// unset as we always have; this isn't great but it's about behaviour preservation.
	ctx.publicAddress, err = f.unit.PublicAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return err
	}
	ctx.privateAddress, err = f.unit.PrivateAddress()
	if err != nil && !params.IsCodeNoAddressSet(err) {
		return err
	}
	return nil
}

func inferRemoteUnit(rctxs map[int]*ContextRelation, info CommandInfo) (int, string, error) {
	relationId := info.RelationId
	hasRelation := relationId != -1
	remoteUnit := info.RemoteUnitName
	hasRemoteUnit := remoteUnit != ""

	// Check baseline sanity of remote unit, if supplied.
	if hasRemoteUnit {
		if !names.IsValidUnit(remoteUnit) {
			return -1, "", errors.Errorf(`invalid remote unit: %s`, remoteUnit)
		} else if !hasRelation {
			return -1, "", errors.Errorf("remote unit provided without a relation: %s", remoteUnit)
		}
	}

	// Check sanity of relation, if supplied, otherwise easy early return.
	if !hasRelation {
		return relationId, remoteUnit, nil
	}
	rctx, found := rctxs[relationId]
	if !found {
		return -1, "", errors.Errorf("unknown relation id: %d", relationId)
	}

	// Past basic sanity checks; if forced, accept what we're given.
	if info.ForceRemoteUnit {
		return relationId, remoteUnit, nil
	}

	// Infer an appropriate remote unit if we can.
	possibles := rctx.UnitNames()
	if remoteUnit == "" {
		switch len(possibles) {
		case 0:
			return -1, "", errors.Errorf("cannot infer remote unit in empty relation %d", relationId)
		case 1:
			return relationId, possibles[0], nil
		}
		return -1, "", errors.Errorf("ambiguous remote unit; possibilities are %+v", possibles)
	}
	for _, possible := range possibles {
		if remoteUnit == possible {
			return relationId, remoteUnit, nil
		}
	}
	return -1, "", errors.Errorf("unknown remote unit %s; possibilities are %+v", remoteUnit, possibles)
}
