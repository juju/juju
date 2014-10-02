// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface
// used by the uniter worker.
package uniter

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Uniter", 0, NewUniterAPI)
}

// UniterAPI implements the API used by the uniter worker.
type UniterAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*common.EnvironWatcher

	st            *state.State
	auth          common.Authorizer
	resources     *common.Resources
	accessUnit    common.GetAuthFunc
	accessService common.GetAuthFunc
}

// NewUniterAPI creates a new instance of the Uniter API.
func NewUniterAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	accessUnit := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	accessService := func() (common.AuthFunc, error) {
		switch tag := authorizer.GetAuthTag().(type) {
		case names.UnitTag:
			entity, err := st.Unit(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			serviceName := entity.ServiceName()
			serviceTag := names.NewServiceTag(serviceName)
			return func(tag names.Tag) bool {
				return tag == serviceTag
			}, nil
		default:
			return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
		}
	}
	accessUnitOrService := common.AuthEither(accessUnit, accessService)
	return &UniterAPI{
		LifeGetter:         common.NewLifeGetter(st, accessUnitOrService),
		StatusSetter:       common.NewStatusSetter(st, accessUnit),
		DeadEnsurer:        common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, accessUnitOrService),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		EnvironWatcher:     common.NewEnvironWatcher(st, resources, authorizer),

		st:            st,
		auth:          authorizer,
		resources:     resources,
		accessUnit:    accessUnit,
		accessService: accessService,
	}, nil
}

func (u *UniterAPI) getUnit(tag names.UnitTag) (*state.Unit, error) {
	return u.st.Unit(tag.Id())
}

func (u *UniterAPI) getService(tag names.ServiceTag) (*state.Service, error) {
	return u.st.Service(tag.Id())
}

// PublicAddress returns the public address for each given unit, if set.
func (u *UniterAPI) PublicAddress(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				address, ok := unit.PublicAddress()
				if ok {
					result.Results[i].Result = address
				} else {
					err = common.NoAddressSetError(tag, "public")
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// PrivateAddress returns the private address for each given unit, if set.
func (u *UniterAPI) PrivateAddress(args params.Entities) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				address, ok := unit.PrivateAddress()
				if ok {
					result.Results[i].Result = address
				} else {
					err = common.NoAddressSetError(tag, "private")
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Resolved returns the current resolved setting for each given unit.
func (u *UniterAPI) Resolved(args params.Entities) (params.ResolvedModeResults, error) {
	result := params.ResolvedModeResults{
		Results: make([]params.ResolvedModeResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ResolvedModeResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].Mode = params.ResolvedMode(unit.Resolved())
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ClearResolved removes any resolved setting from each given unit.
func (u *UniterAPI) ClearResolved(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.ClearResolved()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetPrincipal returns the result of calling PrincipalName() and
// converting it to a tag, on each given unit.
func (u *UniterAPI) GetPrincipal(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				principal, ok := unit.PrincipalName()
				if principal != "" {
					result.Results[i].Result = names.NewUnitTag(principal).String()
				}
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Destroy advances all given Alive units' lifecycles as far as
// possible. See state/Unit.Destroy().
func (u *UniterAPI) Destroy(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.Destroy()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) destroySubordinates(principal *state.Unit) error {
	subordinates := principal.SubordinateNames()
	for _, subName := range subordinates {
		unit, err := u.getUnit(names.NewUnitTag(subName))
		if err != nil {
			return err
		}
		if err = unit.Destroy(); err != nil {
			return err
		}
	}
	return nil
}

// DestroyAllSubordinates destroys all subordinates of each given unit.
func (u *UniterAPI) DestroyAllSubordinates(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = u.destroySubordinates(unit)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// HasSubordinates returns the whether each given unit has any subordinates.
func (u *UniterAPI) HasSubordinates(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.BoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				subordinates := unit.SubordinateNames()
				result.Results[i].Result = len(subordinates) > 0
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmURL returns the charm URL for all given units or services.
func (u *UniterAPI) CharmURL(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	accessUnitOrService := common.AuthEither(u.accessUnit, u.accessService)
	canAccess, err := accessUnitOrService()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unitOrService state.Entity
			unitOrService, err = u.st.FindEntity(tag)
			if err == nil {
				charmURLer := unitOrService.(interface {
					CharmURL() (*charm.URL, bool)
				})
				curl, ok := charmURLer.CharmURL()
				if curl != nil {
					result.Results[i].Result = curl.String()
					result.Results[i].Ok = ok
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetCharmURL sets the charm URL for each given unit. An error will
// be returned if a unit is dead, or the charm URL is not know.
func (u *UniterAPI) SetCharmURL(args params.EntitiesCharmURL) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var curl *charm.URL
				curl, err = charm.ParseURL(entity.CharmURL)
				if err == nil {
					err = unit.SetCharmURL(curl)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// OpenPorts sets the policy of the port range with protocol to be
// opened, for all given units.
func (u *UniterAPI) OpenPorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.OpenPorts(entity.Protocol, entity.FromPort, entity.ToPort)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ClosePorts sets the policy of the port range with protocol to be
// closed, for all given units.
func (u *UniterAPI) ClosePorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				err = unit.ClosePorts(entity.Protocol, entity.FromPort, entity.ToPort)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// OpenPort sets the policy of the port with protocol an number to be
// opened, for all given units.
//
// TODO(dimitern): This is deprecated and is kept for
// backwards-compatibility. Use OpenPorts instead.
func (u *UniterAPI) OpenPort(args params.EntitiesPorts) (params.ErrorResults, error) {
	rangesArgs := params.EntitiesPortRanges{
		Entities: make([]params.EntityPortRange, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		rangesArgs.Entities[i] = params.EntityPortRange{
			Tag:      entity.Tag,
			Protocol: entity.Protocol,
			FromPort: entity.Port,
			ToPort:   entity.Port,
		}
	}
	return u.OpenPorts(rangesArgs)
}

// ClosePort sets the policy of the port with protocol and number to
// be closed, for all given units.
//
// TODO(dimitern): This is deprecated and is kept for
// backwards-compatibility. Use ClosePorts instead.
func (u *UniterAPI) ClosePort(args params.EntitiesPorts) (params.ErrorResults, error) {
	rangesArgs := params.EntitiesPortRanges{
		Entities: make([]params.EntityPortRange, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		rangesArgs.Entities[i] = params.EntityPortRange{
			Tag:      entity.Tag,
			Protocol: entity.Protocol,
			FromPort: entity.Port,
			ToPort:   entity.Port,
		}
	}
	return u.ClosePorts(rangesArgs)
}

func (u *UniterAPI) watchOneUnitConfigSettings(tag names.UnitTag) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	watch, err := unit.WatchConfigSettings()
	if err != nil {
		return "", err
	}
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}
func (u *UniterAPI) watchOneUnitActions(tag names.UnitTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	unit, err := u.getUnit(tag)
	if err != nil {
		return nothing, err
	}
	watch := unit.WatchActions()

	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchConfigSettings returns a NotifyWatcher for observing changes
// to each unit's service configuration settings. See also
// state/watcher.go:Unit.WatchConfigSettings().
func (u *UniterAPI) WatchConfigSettings(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		if canAccess(tag) {
			watcherId, err = u.watchOneUnitConfigSettings(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// WatchMeterStatus returns a NotifyWatcher for observing changes
// to each unit's meter status.
func (u *UniterAPI) WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		if canAccess(tag) {
			watcherId, err = u.watchOneUnitMeterStatus(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneUnitMeterStatus(tag names.UnitTag) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	watch := unit.WatchMeterStatus()
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// WatchActions returns an ActionWatcher for observing incoming action calls
// to a unit.  See also state/watcher.go Unit.WatchActions().  This method
// is called from api/uniter/uniter.go WatchActions().
func (u *UniterAPI) WatchActions(args params.Entities) (params.StringsWatchResults, error) {
	nothing := params.StringsWatchResults{}

	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return nothing, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			return nothing, err
		}
		err = common.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneUnitActions(tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ConfigSettings returns the complete set of service charm config
// settings available to each given unit.
func (u *UniterAPI) ConfigSettings(args params.Entities) (params.ConfigSettingsResults, error) {
	result := params.ConfigSettingsResults{
		Results: make([]params.ConfigSettingsResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ConfigSettingsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				var settings charm.Settings
				settings, err = unit.ConfigSettings()
				if err == nil {
					result.Results[i].Settings = params.ConfigSettings(settings)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneServiceRelations(tag names.ServiceTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	service, err := u.getService(tag)
	if err != nil {
		return nothing, err
	}
	watch := service.WatchRelations()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// WatchServiceRelations returns a StringsWatcher, for each given
// service, that notifies of changes to the lifecycles of relations
// involving that service.
func (u *UniterAPI) WatchServiceRelations(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessService()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseServiceTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			result.Results[i], err = u.watchOneServiceRelations(tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmArchiveSha256 returns the SHA256 digest of the charm archive
// (bundle) data for each charm url in the given parameters.
func (u *UniterAPI) CharmArchiveSha256(args params.CharmURLs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.URLs)),
	}
	for i, arg := range args.URLs {
		curl, err := charm.ParseURL(arg.URL)
		if err != nil {
			err = common.ErrPerm
		} else {
			var sch *state.Charm
			sch, err = u.st.Charm(curl)
			if errors.IsNotFound(err) {
				err = common.ErrPerm
			}
			if err == nil {
				result.Results[i].Result = sch.BundleSha256()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) getRelationAndUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.Relation, *state.Unit, error) {
	tag, err := names.ParseRelationTag(relTag)
	if err != nil {
		return nil, nil, common.ErrPerm
	}
	rel, err := u.st.KeyRelation(tag.Id())
	if errors.IsNotFound(err) {
		return nil, nil, common.ErrPerm
	} else if err != nil {
		return nil, nil, err
	}
	if !canAccess(unitTag) {
		return nil, nil, common.ErrPerm
	}
	unit, err := u.getUnit(unitTag)
	return rel, unit, err
}

func (u *UniterAPI) prepareRelationResult(rel *state.Relation, unit *state.Unit) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	ep, err := rel.Endpoint(unit.ServiceName())
	if err != nil {
		// An error here means the unit's service is not part of the
		// relation.
		return nothing, err
	}
	return params.RelationResult{
		Id:   rel.Id(),
		Key:  rel.String(),
		Life: params.Life(rel.Life().String()),
		Endpoint: params.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		},
	}, nil
}

func (u *UniterAPI) getOneRelation(canAccess common.AuthFunc, relTag, unitTag string) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	tag, err := names.ParseUnitTag(unitTag)
	if err != nil {
		return nothing, common.ErrPerm
	}
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, tag)
	if err != nil {
		return nothing, err
	}
	return u.prepareRelationResult(rel, unit)
}

func (u *UniterAPI) getOneRelationById(relId int) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	rel, err := u.st.Relation(relId)
	if errors.IsNotFound(err) {
		return nothing, common.ErrPerm
	} else if err != nil {
		return nothing, err
	}
	tag := u.auth.GetAuthTag()
	switch tag.(type) {
	case names.UnitTag:
		// do nothing
	default:
		panic("authenticated entity is not a unit")
	}
	unit, err := u.st.FindEntity(tag)
	if err != nil {
		return nothing, err
	}
	// Use the currently authenticated unit to get the endpoint.
	result, err := u.prepareRelationResult(rel, unit.(*state.Unit))
	if err != nil {
		// An error from prepareRelationResult means the authenticated
		// unit's service is not part of the requested
		// relation. That's why it's appropriate to return ErrPerm
		// here.
		return nothing, common.ErrPerm
	}
	return result, nil
}

func (u *UniterAPI) getRelationUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.RelationUnit, error) {
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
	if err != nil {
		return nil, err
	}
	return rel.Unit(unit)
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *UniterAPI) Relation(args params.RelationUnits) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationResults{}, err
	}
	for i, rel := range args.RelationUnits {
		relParams, err := u.getOneRelation(canAccess, rel.Relation, rel.Unit)
		if err == nil {
			result.Results[i] = relParams
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// Actions returns the Actions by Tags passed and ensures that the Unit asking
// for them is the same Unit that has the Actions.
func (u *UniterAPI) Actions(args params.Entities) (params.ActionsQueryResults, error) {
	nothing := params.ActionsQueryResults{}

	actionFn, err := u.authAndActionFromTagFn()
	if err != nil {
		return nothing, err
	}

	results := params.ActionsQueryResults{
		Results: make([]params.ActionsQueryResult, len(args.Entities)),
	}

	for i, arg := range args.Entities {
		action, err := actionFn(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Action.Name = action.Name()
		results.Results[i].Action.Parameters = action.Parameters()
	}

	return results, nil
}

// FinishActions saves the result of a completed Action
func (u *UniterAPI) FinishActions(args params.ActionExecutionResults) (params.ErrorResults, error) {
	nothing := params.ErrorResults{}

	actionFn, err := u.authAndActionFromTagFn()
	if err != nil {
		return nothing, err
	}

	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Results))}

	for i, arg := range args.Results {
		action, err := actionFn(arg.ActionTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		var status state.ActionStatus
		switch arg.Status {
		case "complete":
			status = state.ActionCompleted
		case "fail":
			status = state.ActionFailed
		default:
			results.Results[i].Error = common.ServerError(errors.Errorf("unrecognized action status '%s'", arg.Status))
			continue
		}
		actionResults := state.ActionResults{
			Status:  status,
			Results: arg.Results,
			Message: arg.Message,
		}
		err = action.Finish(actionResults)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return results, nil
}

// authAndActionFromTagFn first authenticates the request, and then returns
// a function with which to authenticate and retrieve each action in the
// request.
func (u *UniterAPI) authAndActionFromTagFn() (func(string) (*state.Action, error), error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return nil, err
	}
	unit, ok := u.auth.GetAuthTag().(names.UnitTag)
	if !ok {
		return nil, fmt.Errorf("calling entity is not a unit")
	}

	return func(tag string) (*state.Action, error) {
		actionTag, err := names.ParseActionTag(tag)
		if err != nil {
			return nil, err
		}

		unitTag := actionTag.PrefixTag()
		if unitTag != unit {
			return nil, common.ErrPerm
		}

		if !canAccess(unitTag) {
			return nil, common.ErrPerm
		}

		return u.st.ActionByTag(actionTag)
	}, nil
}

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint.
func (u *UniterAPI) RelationById(args params.RelationIds) (params.RelationResults, error) {
	result := params.RelationResults{
		Results: make([]params.RelationResult, len(args.RelationIds)),
	}
	for i, relId := range args.RelationIds {
		relParams, err := u.getOneRelationById(relId)
		if err == nil {
			result.Results[i] = relParams
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func relationsInScopeTags(unit *state.Unit) ([]string, error) {
	relations, err := unit.RelationsInScope()
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(relations))
	for i, relation := range relations {
		tags[i] = relation.Tag().String()
	}
	return tags, nil
}

// JoinedRelations returns the tags of all relations for which each supplied unit
// has entered scope. It should be called RelationsInScope, but it's not convenient
// to make that change until we have versioned APIs.
func (u *UniterAPI) JoinedRelations(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canRead, err := u.accessUnit()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canRead(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				result.Results[i].Result, err = relationsInScopeTags(unit)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CurrentEnvironUUID returns the UUID for the current juju environment.
func (u *UniterAPI) CurrentEnvironUUID() (params.StringResult, error) {
	result := params.StringResult{}
	env, err := u.st.Environment()
	if err == nil {
		result.Result = env.UUID()
	}
	return result, err
}

// CurrentEnvironment returns the name and UUID for the current juju environment.
func (u *UniterAPI) CurrentEnvironment() (params.EnvironmentResult, error) {
	result := params.EnvironmentResult{}
	env, err := u.st.Environment()
	if err == nil {
		result.Name = env.Name()
		result.UUID = env.UUID()
	}
	return result, err
}

// ProviderType returns the provider type used by the current juju
// environment.
//
// TODO(dimitern): Refactor the uniter to call this instead of calling
// EnvironConfig() just to get the provider type. Once we have machine
// addresses, this might be completely unnecessary though.
func (u *UniterAPI) ProviderType() (params.StringResult, error) {
	result := params.StringResult{}
	cfg, err := u.st.EnvironConfig()
	if err == nil {
		result.Result = cfg.Type()
	}
	return result, err
}

// EnterScope ensures each unit has entered its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.EnterScope().
func (u *UniterAPI) EnterScope(args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.RelationUnits {
		tag, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, tag)
		if err == nil {
			// Construct the settings, passing the unit's
			// private address (we already know it).
			privateAddress, _ := relUnit.PrivateAddress()
			settings := map[string]interface{}{
				"private-address": privateAddress,
			}
			err = relUnit.EnterScope(settings)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// LeaveScope signals each unit has left its scope in the relation,
// for all of the given relation/unit pairs. See also
// state.RelationUnit.LeaveScope().
func (u *UniterAPI) LeaveScope(args params.RelationUnits) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			err = relUnit.LeaveScope()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func convertRelationSettings(settings map[string]interface{}) (params.RelationSettings, error) {
	result := make(params.RelationSettings)
	for k, v := range settings {
		// All relation settings should be strings.
		sval, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected relation setting %q: expected string, got %T", k, v)
		}
		result[k] = sval
	}
	return result, nil
}

// ReadSettings returns the local settings of each given set of
// relation/unit.
func (u *UniterAPI) ReadSettings(args params.RelationUnits) (params.RelationSettingsResults, error) {
	result := params.RelationSettingsResults{
		Results: make([]params.RelationSettingsResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationSettingsResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			var settings *state.Settings
			settings, err = relUnit.Settings()
			if err == nil {
				result.Results[i].Settings, err = convertRelationSettings(settings.Map())
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag string) (string, error) {
	// Make sure the unit is indeed remote.
	if remoteUnitTag == u.auth.GetAuthTag().String() {
		return "", common.ErrPerm
	}
	// Check remoteUnit is indeed related. Note that we don't want to actually get
	// the *Unit, because it might have been removed; but its relation settings will
	// persist until the relation itself has been removed (and must remain accessible
	// because the local unit's view of reality may be time-shifted).
	tag, err := names.ParseUnitTag(remoteUnitTag)
	if err != nil {
		return "", common.ErrPerm
	}
	remoteUnitName := tag.Id()
	remoteServiceName := names.UnitService(remoteUnitName)
	rel := relUnit.Relation()
	_, err = rel.RelatedEndpoints(remoteServiceName)
	if err != nil {
		return "", common.ErrPerm
	}
	return remoteUnitName, nil
}

// ReadRemoteSettings returns the remote settings of each given set of
// relation/local unit/remote unit.
func (u *UniterAPI) ReadRemoteSettings(args params.RelationUnitPairs) (params.RelationSettingsResults, error) {
	result := params.RelationSettingsResults{
		Results: make([]params.RelationSettingsResult, len(args.RelationUnitPairs)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationSettingsResults{}, err
	}
	for i, arg := range args.RelationUnitPairs {
		unit, err := names.ParseUnitTag(arg.LocalUnit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			// TODO(dfc) rework this logic
			remoteUnit := ""
			remoteUnit, err = u.checkRemoteUnit(relUnit, arg.RemoteUnit)
			if err == nil {
				var settings map[string]interface{}
				settings, err = relUnit.ReadSettings(remoteUnit)
				if err == nil {
					result.Results[i].Settings, err = convertRelationSettings(settings)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// UpdateSettings persists all changes made to the local settings of
// all given pairs of relation and unit. Keys with empty values are
// considered a signal to delete these values.
func (u *UniterAPI) UpdateSettings(args params.RelationUnitsSettings) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			var settings *state.Settings
			settings, err = relUnit.Settings()
			if err == nil {
				for k, v := range arg.Settings {
					if v == "" {
						settings.Delete(k)
					} else {
						settings.Set(k, v)
					}
				}
				_, err = settings.Write()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneRelationUnit(relUnit *state.RelationUnit) (params.RelationUnitsWatchResult, error) {
	watch := relUnit.Watch()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.RelationUnitsWatchResult{
			RelationUnitsWatcherId: u.resources.Register(watch),
			Changes:                changes,
		}, nil
	}
	return params.RelationUnitsWatchResult{}, watcher.EnsureErr(watch)
}

// WatchRelationUnits returns a RelationUnitsWatcher for observing
// changes to every unit in the supplied relation that is visible to
// the supplied unit. See also state/watcher.go:RelationUnit.Watch().
func (u *UniterAPI) WatchRelationUnits(args params.RelationUnits) (params.RelationUnitsWatchResults, error) {
	result := params.RelationUnitsWatchResults{
		Results: make([]params.RelationUnitsWatchResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.RelationUnitsWatchResults{}, err
	}
	for i, arg := range args.RelationUnits {
		unit, err := names.ParseUnitTag(arg.Unit)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, unit)
		if err == nil {
			result.Results[i], err = u.watchOneRelationUnit(relUnit)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// TODO(dimitern) bug #1270795 2014-01-20
// Add a doc comment here and use u.accessService()
// below in the body to check for permissions.
func (u *UniterAPI) GetOwnerTag(args params.Entities) (params.StringResult, error) {
	var nothing params.StringResult
	tag, err := names.ParseServiceTag(args.Entities[0].Tag)
	if err != nil {
		return nothing, common.ErrPerm
	}
	service, err := u.getService(tag)
	if err != nil {
		return nothing, err
	}
	return params.StringResult{
		Result: service.GetOwnerTag(),
	}, nil
}

func (u *UniterAPI) watchOneUnitAddresses(tag names.UnitTag) (string, error) {
	unit, err := u.getUnit(tag)
	if err != nil {
		return "", err
	}
	machineId, err := unit.AssignedMachineId()
	if err != nil {
		return "", err
	}
	machine, err := u.st.Machine(machineId)
	if err != nil {
		return "", err
	}
	watch := machine.WatchAddresses()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// WatchAddresses returns a NotifyWatcher for observing changes
// to each unit's addresses.
func (u *UniterAPI) WatchUnitAddresses(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		unit, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		watcherId := ""
		if canAccess(unit) {
			watcherId, err = u.watchOneUnitAddresses(unit)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// AddMetrics adds the metrics for the specified unit.
func (u *UniterAPI) AddMetrics(args params.MetricsParams) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Metrics)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, common.ErrPerm
	}
	for i, unitMetrics := range args.Metrics {
		tag, err := names.ParseUnitTag(unitMetrics.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var unit *state.Unit
			unit, err = u.getUnit(tag)
			if err == nil {
				metricBatch := make([]state.Metric, len(unitMetrics.Metrics))
				for j, metric := range unitMetrics.Metrics {
					// TODO (tasdomas) 2014-08-26: set credentials for metrics when available
					metricBatch[j] = state.Metric{
						Key:   metric.Key,
						Value: metric.Value,
						Time:  metric.Time,
					}
				}
				_, err = unit.AddMetrics(time.Now(), metricBatch)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetMeterStatus returns meter status information for each unit.
func (u *UniterAPI) GetMeterStatus(args params.Entities) (params.MeterStatusResults, error) {
	result := params.MeterStatusResults{
		Results: make([]params.MeterStatusResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.MeterStatusResults{}, common.ErrPerm
	}
	for i, entity := range args.Entities {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		var code string
		var info string
		if canAccess(unitTag) {
			var unit *state.Unit
			unit, err = u.getUnit(unitTag)
			if err == nil {
				code, info, err = unit.GetMeterStatus()
			}
		}
		result.Results[i].Code = code
		result.Results[i].Info = info
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
