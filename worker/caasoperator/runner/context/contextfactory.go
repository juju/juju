// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/worker/caasoperator/hook"
)

// CommandInfo specifies the information necessary to run a command.
type CommandInfo struct {
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// ForceRemoteUnit skips unit inference and existence validation.
	ForceRemoteUnit bool
}

// ContextFactory represents a long-lived object that can create execution contexts
// relevant to a specific unit.
type ContextFactory interface {
	// CommandContext creates a new context for running a juju command.
	CommandContext(commandInfo CommandInfo) (*HookContext, error)

	// HookContext creates a new context for running a juju hook.
	HookContext(hookInfo hook.Info) (*HookContext, error)
}

// RelationsFunc is used to get snapshots of relation membership at context
// creation time.
type RelationsFunc func() map[int]*RelationInfo

type contextFactory struct {
	contextFactoryAPI contextFactoryAPI
	hookAPI           hookAPI

	// Fields that shouldn't change in a factory's lifetime.
	applicationTag names.ApplicationTag
	paths          Paths
	modelUUID      string
	modelName      string
	clock          clock.Clock

	// Callback to get relation state snapshot.
	getRelationInfos RelationsFunc
	relationCaches   map[int]*RelationCache

	// For generating "unique" context ids.
	rand *rand.Rand
}

// FactoryConfig contains configuration values
// for the context factory.
type FactoryConfig struct {
	// These API attributes define the interface the backend.
	ContextFactoryAPI contextFactoryAPI
	HookAPI           hookAPI
	GetRelationInfos  RelationsFunc

	ModelUUID      string
	ModelName      string
	ApplicationTag names.ApplicationTag
	Paths          Paths
	Clock          clock.Clock
}

// NewContextFactory returns a ContextFactory capable of creating execution contexts backed
// by the supplied unit's supplied API connection.
func NewContextFactory(config FactoryConfig) (ContextFactory, error) {
	f := &contextFactory{
		contextFactoryAPI: config.ContextFactoryAPI,
		hookAPI:           config.HookAPI,
		applicationTag:    config.ApplicationTag,
		paths:             config.Paths,
		modelUUID:         config.ModelUUID,
		modelName:         config.ModelName,
		getRelationInfos:  config.GetRelationInfos,
		relationCaches:    map[int]*RelationCache{},
		rand:              rand.New(rand.NewSource(time.Now().Unix())),
		clock:             config.Clock,
	}
	return f, nil
}

// newId returns a probably-unique identifier for a new context, containing the
// supplied string.
func (f *contextFactory) newId(name string) string {
	return fmt.Sprintf("%s-%s-%d", f.applicationTag.Id(), name, f.rand.Int63())
}

// coreContext creates a new context with all unspecialised fields filled in.
func (f *contextFactory) coreContext() (*HookContext, error) {
	ctx := &HookContext{
		hookAPI:         f.hookAPI,
		uuid:            f.modelUUID,
		modelName:       f.modelName,
		applicationName: f.applicationTag.Id(),
		relations:       f.getContextRelations(),
		relationId:      -1,
		clock:           f.clock,
	}
	if err := f.updateContext(ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// HookContext is part of the ContextFactory interface.
func (f *contextFactory) HookContext(hookInfo hook.Info) (*HookContext, error) {
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
		// TODO(caas) - we don't run departed hooks so figure out when to clear cache
		if hookInfo.Kind == hooks.RelationDeparted {
			relation.cache.RemoveMember(hookInfo.RemoteUnit)
		} else if hookInfo.RemoteUnit != "" {
			// Clear remote settings cache for changing remote unit.
			relation.cache.InvalidateMember(hookInfo.RemoteUnit)
		}
		hookName = fmt.Sprintf("%s-%s", relation.Name(), hookInfo.Kind)
	}
	ctx.id = f.newId(hookName)
	return ctx, nil
}

// CommandContext is part of the ContextFactory interface.
func (f *contextFactory) CommandContext(commandInfo CommandInfo) (*HookContext, error) {
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
	return ctx, nil
}

// getContextRelations updates the factory's relation caches, and uses them
// to construct ContextRelations for a fresh context.
func (f *contextFactory) getContextRelations() map[int]*ContextRelation {
	contextRelations := map[int]*ContextRelation{}
	relationInfos := f.getRelationInfos()
	relationCaches := map[int]*RelationCache{}
	for id, info := range relationInfos {
		memberNames := info.MemberNames
		cache, found := f.relationCaches[id]
		if found {
			cache.Prune(memberNames)
		} else {
			cache = NewRelationCache(info.RelationUnitAPI.RemoteSettings, memberNames)
		}
		relationCaches[id] = cache
		contextRelations[id] = NewContextRelation(info.RelationUnitAPI, cache)
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
func (f *contextFactory) updateContext(ctx *HookContext) (err error) {
	defer errors.Trace(err)

	ctx.apiAddrs, err = f.contextFactoryAPI.APIAddresses()
	if err != nil {
		return err
	}

	proxySettings, err := f.contextFactoryAPI.ProxySettings()
	if err != nil {
		return err
	}
	ctx.proxySettings = proxySettings

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
