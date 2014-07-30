// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface
// used by the uniter worker.
package uniter

import (
	"fmt"

	"github.com/juju/charm"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
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
		unit, ok := authorizer.GetAuthEntity().(*state.Unit)
		if !ok {
			panic("authenticated entity is not a unit")
		}
		return func(tag string) bool {
			return tag == names.NewServiceTag(unit.ServiceName()).String()
		}, nil
	}
	accessUnitOrService := common.AuthEither(accessUnit, accessService)
	// Uniter can always watch for environ changes.
	getCanWatch := common.AuthAlways(true)
	// Uniter can not get the secrets.
	getCanReadSecrets := common.AuthAlways(false)
	return &UniterAPI{
		LifeGetter:         common.NewLifeGetter(st, accessUnitOrService),
		StatusSetter:       common.NewStatusSetter(st, accessUnit),
		DeadEnsurer:        common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher: common.NewAgentEntityWatcher(st, resources, accessUnitOrService),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		EnvironWatcher:     common.NewEnvironWatcher(st, resources, getCanWatch, getCanReadSecrets),

		st:            st,
		auth:          authorizer,
		resources:     resources,
		accessUnit:    accessUnit,
		accessService: accessService,
	}, nil
}

func (u *UniterAPI) getUnit(tag string) (*state.Unit, error) {
	t, err := names.ParseUnitTag(tag)
	if err != nil {
		return nil, err
	}
	return u.st.Unit(t.Id())
}

func (u *UniterAPI) getService(tag string) (*state.Service, error) {
	t, err := names.ParseServiceTag(tag)
	if err != nil {
		return nil, err
	}
	return u.st.Service(t.Id())
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				address, ok := unit.PublicAddress()
				if ok {
					result.Results[i].Result = address
				} else {
					err = common.NoAddressSetError(entity.Tag, "public")
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
			if err == nil {
				address, ok := unit.PrivateAddress()
				if ok {
					result.Results[i].Result = address
				} else {
					err = common.NoAddressSetError(entity.Tag, "private")
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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
		unit, err := u.getUnit(names.NewUnitTag(subName).String())
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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
func (u *UniterAPI) watchOneUnitActions(tag string) (params.StringsWatchResult, error) {
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
	return nothing, watcher.MustErr(watch)
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

// WatchActions returns an ActionWatcher for observing incoming action calls
// to a unit.  See also state/watcher.go Unit.WatchActions().  This method
// is called from state/api/uniter/uniter.go WatchActions().
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
		_, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			return nothing, err
		}

		err = common.ErrPerm
		if canAccess(entity.Tag) {
			result.Results[i], err = u.watchOneUnitActions(entity.Tag)
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
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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

// CharmArchiveURL returns the URL, corresponding to the charm archive
// (bundle) in the provider storage for each given charm URL, along
// with the DisableSSLHostnameVerification flag.
func (u *UniterAPI) CharmArchiveURL(args params.CharmURLs) (params.CharmArchiveURLResults, error) {
	result := params.CharmArchiveURLResults{
		Results: make([]params.CharmArchiveURLResult, len(args.URLs)),
	}
	// Get the SSL hostname verification environment setting.
	envConfig, err := u.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	// SSLHostnameVerification defaults to true, so we need to
	// invert that, for backwards-compatibility (older versions
	// will have DisableSSLHostnameVerification: false by default).
	disableSSLHostnameVerification := !envConfig.SSLHostnameVerification()
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
				result.Results[i].Result = sch.BundleURL().String()
				result.Results[i].DisableSSLHostnameVerification = disableSSLHostnameVerification
			}
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

func (u *UniterAPI) getRelationAndUnit(canAccess common.AuthFunc, relTag, unitTag string) (*state.Relation, *state.Unit, error) {
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
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
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
	// Use the currently authenticated unit to get the endpoint.
	unit, ok := u.auth.GetAuthEntity().(*state.Unit)
	if !ok {
		panic("authenticated entity is not a unit")
	}
	result, err := u.prepareRelationResult(rel, unit)
	if err != nil {
		// An error from prepareRelationResult means the authenticated
		// unit's service is not part of the requested
		// relation. That's why it's appropriate to return ErrPerm
		// here.
		return nothing, common.ErrPerm
	}
	return result, nil
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

// getOneActionByTag retrieves a single Action by Tag.
func (u *UniterAPI) getOneActionByTag(tag names.ActionTag) (params.ActionsQueryResult, error) {
	result := params.ActionsQueryResult{}
	action, err := u.st.ActionByTag(tag)
	if err != nil {
		return result, err
	}

	result.Action = &params.Action{
		Name:   action.Name(),
		Params: action.Payload(),
	}
	return result, nil
}

// Actions returns the Actions by Tags passed and ensures that the Unit asking
// for them is the same Unit that has the Actions.
func (u *UniterAPI) Actions(args params.Entities) (params.ActionsQueryResults, error) {
	nothing := params.ActionsQueryResults{}

	canAccess, err := u.accessUnit()
	if err != nil {
		return nothing, err
	}

	results := params.ActionsQueryResults{
		ActionsQueryResults: make([]params.ActionsQueryResult, len(args.Entities)),
	}

	for i, actionQuery := range args.Entities {
		// Use the currently authenticated unit to get the endpoint.
		whichUnit, ok := u.auth.GetAuthEntity().(*state.Unit)
		if !ok {
			return nothing, fmt.Errorf("entity is not a unit")
		}

		// this Unit must match the Action's prefix.
		actionTag, err := names.ParseActionTag(actionQuery.Tag)
		if err != nil {
			return nothing, err
		}
		unitTag := actionTag.PrefixTag()

		// The Unit is querying for another Unit's Action.
		if unitTag.String() != whichUnit.Tag().String() {
			return nothing, common.ErrPerm
		}

		// The Unit does not have access.
		if !canAccess(unitTag.String()) {
			return nothing, common.ErrPerm
		}

		actionQueryResult, err := u.getOneActionByTag(actionTag)
		if err == nil {
			results.ActionsQueryResults[i] = actionQueryResult
		}
		results.ActionsQueryResults[i].Error = common.ServerError(err)
	}

	return results, nil
}

// ActionComplete saves the result of a completed Action
func (u *UniterAPI) ActionComplete(args params.ActionResult) (params.BoolResult, error) {
	action, err := u.actionIfPermitted(args.ActionTag)
	if err == nil {
		err = action.Complete(args.Output)
	}
	return params.BoolResult{Error: common.ServerError(err), Result: err == nil}, err
}

// ActionFail saves the result of a completed Action
func (u *UniterAPI) ActionFail(args params.ActionResult) (params.BoolResult, error) {
	action, err := u.actionIfPermitted(args.ActionTag)
	if err == nil {
		err = action.Fail(args.Output)
	}
	return params.BoolResult{Error: common.ServerError(err), Result: err == nil}, err
}

// actionIfPermitted returns an action, only if canAccess permits,
// returns common.ErrPerm if not permitted
func (u *UniterAPI) actionIfPermitted(tag string) (*state.Action, error) {
	canAccess, err := u.accessUnit()
	if err != nil {
		return nil, err
	}
	// Use the currently authenticated unit to get the endpoint.
	whichUnit, ok := u.auth.GetAuthEntity().(*state.Unit)
	if !ok {
		return nil, fmt.Errorf("entity is not a unit")
	}

	// this Unit must match the Action's prefix.
	actionTag, err := names.ParseActionTag(tag)
	if err != nil {
		return nil, err
	}
	unitTag := actionTag.PrefixTag()

	// The Unit is querying for another Unit's Action.
	if unitTag.String() != whichUnit.Tag().String() {
		return nil, common.ErrPerm
	}

	// The Unit does not have access.
	if !canAccess(unitTag.String()) {
		return nil, common.ErrPerm
	}

	return u.st.ActionByTag(actionTag)
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
		err := common.ErrPerm
		if canRead(entity.Tag) {
			var unit *state.Unit
			unit, err = u.getUnit(entity.Tag)
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
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
		return "", err
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.LocalUnit)
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
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
	return params.RelationUnitsWatchResult{}, watcher.MustErr(watch)
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
		relUnit, err := u.getRelationUnit(canAccess, arg.Relation, arg.Unit)
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

	nothing := params.StringResult{}
	service, err := u.getService(args.Entities[0].Tag)
	if err != nil {
		return nothing, err
	}

	return params.StringResult{
		Result: service.GetOwnerTag(),
	}, nil
}

func (u *UniterAPI) watchOneUnitAddresses(tag string) (string, error) {
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
	return "", watcher.MustErr(watch)
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
		err := common.ErrPerm
		watcherId := ""
		if canAccess(entity.Tag) {
			watcherId, err = u.watchOneUnitAddresses(entity.Tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
