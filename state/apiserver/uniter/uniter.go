// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface
// used by the uniter worker.
package uniter

import (
	"strconv"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
)

// UniterAPI implements the API used by the uniter worker.
type UniterAPI struct {
	*common.LifeGetter
	*common.StatusSetter
	*common.DeadEnsurer
	*common.AgentEntityWatcher

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
		unit, ok := authorizer.GetAuthEntity().(*state.Unit)
		if !ok {
			panic("authenticated entity is not a unit")
		}
		return func(tag string) bool {
			return tag == names.ServiceTag(unit.ServiceName())
		}, nil
	}
	accessRelation := func() (common.AuthFunc, error) {
		return func(tag string) bool {
			_, _, err := names.ParseTag(tag, names.RelationTagKind)
			return err == nil
		}, nil
	}
	accessUnitOrService := common.AuthEither(accessUnit, accessService)
	accessUnitServiceOrRelation := common.AuthEither(accessUnitOrService, accessRelation)
	return &UniterAPI{
		LifeGetter:         common.NewLifeGetter(st, accessUnitServiceOrRelation),
		StatusSetter:       common.NewStatusSetter(st, accessUnit),
		DeadEnsurer:        common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, accessUnitOrService),
		st:                 st,
		auth:               authorizer,
		resources:          resources,
		accessUnit:         accessUnit,
		accessService:      accessService,
	}, nil
}

func (u *UniterAPI) getUnit(tag string) (*state.Unit, error) {
	entity, err := u.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Unit), nil
}

func (u *UniterAPI) getService(tag string) (*state.Service, error) {
	entity, err := u.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Service), nil
}

// PublicAddress returns for each given unit, a pair of the public
// address of the unit and whether it's valid.
func (u *UniterAPI) PublicAddress(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				address, ok := unit.PublicAddress()
				result.Results[i].Result = address
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetPrivateAddress sets the public address of each of the given units.
func (u *UniterAPI) SetPublicAddress(args params.SetEntityAddresses) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.SetPublicAddress(entity.Address)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// PrivateAddress returns for each given unit, a pair of the private
// address of the unit and whether it's valid.
func (u *UniterAPI) PrivateAddress(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				address, ok := unit.PrivateAddress()
				result.Results[i].Result = address
				result.Results[i].Ok = ok
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SetPrivateAddress sets the private address of each of the given units.
func (u *UniterAPI) SetPrivateAddress(args params.SetEntityAddresses) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.SetPrivateAddress(entity.Address)
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.ClearResolved()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// GetPrincipal returns the result of calling PrincipalName() on each given unit.
func (u *UniterAPI) GetPrincipal(args params.Entities) (params.StringBoolResults, error) {
	result := params.StringBoolResults{
		Results: make([]params.StringBoolResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				principal, ok := unit.PrincipalName()
				result.Results[i].Result = principal
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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
		unit, err := u.getUnit(names.UnitTag(subName))
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = u.destroySubordinates(unit)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// SubordinateNames returns the names of any subordinate units, for each given unit.
func (u *UniterAPI) SubordinateNames(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringsResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				result.Results[i].Result = unit.SubordinateNames()
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unitOrService state.Entity
			unitOrService, err = u.st.FindEntity(entity.Tag)
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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

// OpenPort sets the policy of the port with protocol an number to be
// opened, for all given units.
func (u *UniterAPI) OpenPort(args params.EntitiesPorts) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.OpenPort(entity.Protocol, entity.Port)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ClosePort sets the policy of the port with protocol and number to
// be closed, for all given units.
func (u *UniterAPI) ClosePort(args params.EntitiesPorts) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.ErrorResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				err = unit.ClosePort(entity.Protocol, entity.Port)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneUnitConfigSettings(tag string) (string, error) {
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
	return "", watcher.MustErr(watch)
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
		err := common.ErrPerm
		watcherId := ""
		if canAccess(entity.Tag) {
			watcherId, err = u.watchOneUnitConfigSettings(entity.Tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ConfigSettings returns the complete set of service charm config
// settings available to each given unit.
func (u *UniterAPI) ConfigSettings(args params.Entities) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				var settings charm.Settings
				settings, err = unit.ConfigSettings()
				if err == nil {
					result.Results[i].Settings = convertSettings(settings)
				}
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) watchOneServiceRelations(tag string) (params.StringsWatchResult, error) {
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
	return nothing, watcher.MustErr(watch)
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			result.Results[i], err = u.watchOneServiceRelations(entity.Tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmBundleURL returns the URL, corresponding to the charm bundle
// in the provider storage for each given charm URL.
func (u *UniterAPI) CharmBundleURL(args params.CharmURLs) (params.StringResults, error) {
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
			if errors.IsNotFoundError(err) {
				err = common.ErrPerm
			}
			if err == nil {
				result.Results[i].Result = sch.BundleURL().String()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CharmBundleURL returns the SHA256 digest of the charm bundle data
// for each charm url in the given parameters.
func (u *UniterAPI) CharmBundleSha256(args params.CharmURLs) (params.StringResults, error) {
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
			if errors.IsNotFoundError(err) {
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

func (u *UniterAPI) getRelationAndUnit(canAccess common.AuthFunc, relTag, unitTag string) (*state.Relation, *state.Unit, error) {
	_, id, err := names.ParseTag(relTag, names.RelationTagKind)
	if err != nil {
		return nil, nil, common.ErrPerm
	}
	// TODO(dimitern): Once the relation tags have a different format
	// (e.g. "relation-service1@name1+service2@name2"), change the
	// following code accordingly.
	relId, err := strconv.Atoi(id)
	if err != nil {
		return nil, nil, common.ErrPerm
	}
	rel, err := u.st.Relation(relId)
	if errors.IsNotFoundError(err) {
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

func (u *UniterAPI) getOneRelation(canAccess common.AuthFunc, relTag, unitTag string) (params.RelationResult, error) {
	nothing := params.RelationResult{}
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
	if err != nil {
		return nothing, err
	}
	ep, err := rel.Endpoint(unit.ServiceName())
	if err != nil {
		return nothing, err
	}
	return params.RelationResult{
		Id:  rel.Id(),
		Key: rel.String(),
		Endpoint: params.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		},
	}, nil
}

func (u *UniterAPI) getRelationUnit(canAccess common.AuthFunc, relTag, unitTag string) (*state.RelationUnit, error) {
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

// CurrentEnvironUUID returns the UUID for the current juju environment.
func (u *UniterAPI) CurrentEnvironUUID() (params.StringResult, error) {
	result := params.StringResult{}
	env, err := u.st.Environment()
	if err == nil {
		result.Result = env.UUID()
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
		if err == nil {
			err = relUnit.LeaveScope()
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func convertSettings(settings map[string]interface{}) params.Settings {
	result := make(params.Settings)
	for k, v := range settings {
		// All relation settings should be strings.
		sval, _ := v.(string)
		result[k] = sval
	}
	return result
}

// ReadSettings returns the local settings of each given set of
// relation/unit.
func (u *UniterAPI) ReadSettings(args params.RelationUnits) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
	}
	for i, arg := range args.RelationUnits {
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
		if err == nil {
			var settings *state.Settings
			settings, err = relUnit.Settings()
			if err == nil {
				result.Results[i].Settings = convertSettings(settings.Map())
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UniterAPI) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag string) (string, error) {
	// Make sure the unit is indeed remote.
	if remoteUnitTag == u.auth.GetAuthTag() {
		return "", common.ErrPerm
	}
	remoteUnit, err := u.getUnit(remoteUnitTag)
	if err != nil {
		return "", common.ErrPerm
	}
	// Check remoteUnit is indeed related.
	rel := relUnit.Relation()
	_, err = rel.RelatedEndpoints(remoteUnit.ServiceName())
	if err != nil {
		return "", common.ErrPerm
	}
	return remoteUnit.Name(), nil
}

// ReadRemoteSettings returns the remote settings of each given set of
// relation/local unit/remote unit.
func (u *UniterAPI) ReadRemoteSettings(args params.RelationUnitPairs) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnitPairs)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
	}
	for i, arg := range args.RelationUnitPairs {
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.LocalUnit)
		if err == nil {
			remoteUnit := ""
			remoteUnit, err = u.checkRemoteUnit(relUnit, arg.RemoteUnit)
			if err == nil {
				var settings map[string]interface{}
				settings, err = relUnit.ReadSettings(remoteUnit)
				if err == nil {
					result.Results[i].Settings = convertSettings(settings)
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
		if err == nil {
			var settings *state.Settings
			settings, err = relUnit.Settings()
			if err == nil {
				mapSettings := make(map[string]interface{})
				for _, k := range settings.Keys() {
					if v, ok := arg.Settings[k]; ok && v == "" {
						settings.Delete(k)
						delete(arg.Settings, k)
					} else {
						mapSettings[k] = arg.Settings[k]
					}
				}
				settings.Update(mapSettings)
				_, err = settings.Write()
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// TODO(dimitern): Add the following needed calls/features:
// RelationUnitsWatcher
