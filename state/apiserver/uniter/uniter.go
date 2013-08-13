// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface
// used by the uniter worker.
package uniter

import (
	"fmt"

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

	st                        *state.State
	auth                      common.Authorizer
	resources                 *common.Resources
	getCanAccessUnit          common.GetAuthFunc
	getCanAccessService       common.GetAuthFunc
	getCanAccessUnitOrService common.GetAuthFunc
}

// NewUniterAPI creates a new instance of the Uniter API.
func NewUniterAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	getCanAccessUnit := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	getCanAccessService := func() (common.AuthFunc, error) {
		unit, ok := authorizer.GetAuthEntity().(*state.Unit)
		if !ok {
			panic("authenticated entity is not a unit")
		}
		return func(tag string) bool {
			return tag == names.ServiceTag(unit.ServiceName())
		}, nil
	}
	getCanAccessUnitOrService := func() (common.AuthFunc, error) {
		// These errors here are ignored: they're always nil.
		canAccessService, _ := getCanAccessService()
		canAccessUnit, _ := getCanAccessUnit()
		return func(tag string) bool {
			if !canAccessService(tag) {
				return canAccessUnit(tag)
			}
			return true
		}, nil
	}
	return &UniterAPI{
		LifeGetter:                common.NewLifeGetter(st, getCanAccessUnitOrService),
		StatusSetter:              common.NewStatusSetter(st, getCanAccessUnit),
		DeadEnsurer:               common.NewDeadEnsurer(st, getCanAccessUnit),
		AgentEntityWatcher:        common.NewAgentEntityWatcher(st, resources, getCanAccessUnitOrService),
		st:                        st,
		auth:                      authorizer,
		resources:                 resources,
		getCanAccessUnit:          getCanAccessUnit,
		getCanAccessService:       getCanAccessService,
		getCanAccessUnitOrService: getCanAccessUnitOrService,
	}, nil
}

func (u *UniterAPI) getUnitOrService(tag string) (state.Entity, error) {
	kind, err := names.TagKind(tag)
	if err != nil {
		// Invalid tags might indicate something security related.
		return nil, common.ErrPerm
	}
	if kind != names.UnitTagKind && kind != names.ServiceTagKind {
		return nil, fmt.Errorf("%q is not a unit or a service tag", tag)
	}
	return u.st.FindEntity(tag)
}

func (u *UniterAPI) getUnit(tag string) (*state.Unit, error) {
	entity, err := u.getUnitOrService(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Unit), nil
}

func (u *UniterAPI) getService(tag string) (*state.Service, error) {
	entity, err := u.getUnitOrService(tag)
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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

// ClearResolved removes any resolved setting from each given unit.
func (u *UniterAPI) ClearResolved(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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

// SubordinateNames returns the names of any subordinate units, for each given unit.
func (u *UniterAPI) SubordinateNames(args params.Entities) (params.StringsResults, error) {
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.Entities)),
	}
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnitOrService()
	if err != nil {
		return params.StringBoolResults{}, err
	}
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if canAccess(entity.Tag) {
			var unitOrService interface{}
			unitOrService, err = u.getUnitOrService(entity.Tag)
			if err == nil {
				charmURLer := unitOrService.(interface {
					CharmURL() (*charm.URL, bool)
				})
				curl, ok := charmURLer.CharmURL()
				result.Results[i].Result = curl.String()
				result.Results[i].Ok = ok
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
	canAccess, err := u.getCanAccessUnit()
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
					result.Results[i].Settings = params.Settings(settings)
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
	canAccess, err := u.getCanAccessService()
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

// TODO(dimitern): Add the following needed calls:
// RelationSetDying
// RelationIsImplicit
// RelationDestroy
// RelationRelationUnit
// RelationUnitRelation
// RelationUnitSettings
// RelationUnitEnterScope
// RelationUnitLeaveScope
// RelationUnitEndpoint
// EndpointImplementedBy
// EnvironUUID
