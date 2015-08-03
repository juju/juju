// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface used by the uniter
// worker. This file contains code common to all API versions.
package uniter

import (
	"fmt"
	"net/url"
	"path"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	leadershipapiserver "github.com/juju/juju/apiserver/leadership"
	"github.com/juju/juju/apiserver/meterstatus"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/leadership"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

// uniterBaseAPI implements common methods used by all API versions,
// and it's intended for embedding.
type uniterBaseAPI struct {
	*common.LifeGetter
	*StatusAPI
	*common.DeadEnsurer
	*common.AgentEntityWatcher
	*common.APIAddresser
	*common.EnvironWatcher
	*common.RebootRequester
	*leadershipapiserver.LeadershipSettingsAccessor

	st            *state.State
	auth          common.Authorizer
	resources     *common.Resources
	accessUnit    common.GetAuthFunc
	accessService common.GetAuthFunc
	unit          *state.Unit
}

// newUniterBaseAPI creates a new instance of the uniter base API.
func newUniterBaseAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*uniterBaseAPI, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	var unit *state.Unit
	var err error
	switch tag := authorizer.GetAuthTag().(type) {
	case names.UnitTag:
		unit, err = st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
	default:
		return nil, errors.Errorf("expected names.UnitTag, got %T", tag)
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
	accessMachine := func() (common.AuthFunc, error) {
		machineId, err := unit.AssignedMachineId()
		if err != nil {
			return nil, errors.Trace(err)
		}
		machine, err := st.Machine(machineId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return func(tag names.Tag) bool {
			return tag == machine.Tag()
		}, nil
	}

	accessUnitOrService := common.AuthEither(accessUnit, accessService)
	return &uniterBaseAPI{
		LifeGetter:                 common.NewLifeGetter(st, accessUnitOrService),
		DeadEnsurer:                common.NewDeadEnsurer(st, accessUnit),
		AgentEntityWatcher:         common.NewAgentEntityWatcher(st, resources, accessUnitOrService),
		APIAddresser:               common.NewAPIAddresser(st, resources),
		EnvironWatcher:             common.NewEnvironWatcher(st, resources, authorizer),
		RebootRequester:            common.NewRebootRequester(st, accessMachine),
		LeadershipSettingsAccessor: leadershipSettingsAccessorFactory(st, resources, authorizer),

		// TODO(fwereade): so *every* unit should be allowed to get/set its
		// own status *and* its service's? This is not a pleasing arrangement.
		StatusAPI: NewStatusAPI(st, accessUnitOrService),

		st:            st,
		auth:          authorizer,
		resources:     resources,
		accessUnit:    accessUnit,
		accessService: accessService,
		unit:          unit,
	}, nil
}

// PublicAddress returns the public address for each given unit, if set.
func (u *uniterBaseAPI) PublicAddress(args params.Entities) (params.StringResults, error) {
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
func (u *uniterBaseAPI) PrivateAddress(args params.Entities) (params.StringResults, error) {
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

// TODO(ericsnow) Factor out the common code amongst the many methods here.

var getZone = func(st *state.State, tag names.Tag) (string, error) {
	unit, err := st.Unit(tag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	zone, err := unit.AvailabilityZone()
	return zone, errors.Trace(err)
}

// AvailabilityZone returns the availability zone for each given unit, if applicable.
func (u *uniterBaseAPI) AvailabilityZone(args params.Entities) (params.StringResults, error) {
	var results params.StringResults

	canAccess, err := u.accessUnit()
	if err != nil {
		return results, errors.Trace(err)
	}

	// Prep the results.
	results = params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}

	// Collect the zones. No zone will be collected for any entity where
	// the tag is invalid or not authorized. Instead the corresponding
	// result will be updated with the error.
	for i, entity := range args.Entities {
		tag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		if canAccess(tag) {
			var zone string
			zone, err = getZone(u.st, tag)
			if err == nil {
				results.Results[i].Result = zone
			}
		}
		results.Results[i].Error = common.ServerError(err)
	}

	return results, nil
}

// Resolved returns the current resolved setting for each given unit.
func (u *uniterBaseAPI) Resolved(args params.Entities) (params.ResolvedModeResults, error) {
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
func (u *uniterBaseAPI) ClearResolved(args params.Entities) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) GetPrincipal(args params.Entities) (params.StringBoolResults, error) {
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
func (u *uniterBaseAPI) Destroy(args params.Entities) (params.ErrorResults, error) {
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

// DestroyAllSubordinates destroys all subordinates of each given unit.
func (u *uniterBaseAPI) DestroyAllSubordinates(args params.Entities) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) HasSubordinates(args params.Entities) (params.BoolResults, error) {
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
func (u *uniterBaseAPI) CharmURL(args params.Entities) (params.StringBoolResults, error) {
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
func (u *uniterBaseAPI) SetCharmURL(args params.EntitiesCharmURL) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) OpenPorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) ClosePorts(args params.EntitiesPortRanges) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) OpenPort(args params.EntitiesPorts) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) ClosePort(args params.EntitiesPorts) (params.ErrorResults, error) {
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

// WatchConfigSettings returns a NotifyWatcher for observing changes
// to each unit's service configuration settings. See also
// state/watcher.go:Unit.WatchConfigSettings().
func (u *uniterBaseAPI) WatchConfigSettings(args params.Entities) (params.NotifyWatchResults, error) {
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
func (u *uniterBaseAPI) WatchMeterStatus(args params.Entities) (params.NotifyWatchResults, error) {
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

// WatchActionNotifications returns a StringsWatcher for observing
// incoming action calls to a unit. See also state/watcher.go
// Unit.WatchActionNotifications(). This method is called from
// api/uniter/uniter.go WatchActionNotifications().
func (u *uniterBaseAPI) WatchActionNotifications(args params.Entities) (params.StringsWatchResults, error) {
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
			result.Results[i], err = u.watchOneUnitActionNotifications(tag)
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// ConfigSettings returns the complete set of service charm config
// settings available to each given unit.
func (u *uniterBaseAPI) ConfigSettings(args params.Entities) (params.ConfigSettingsResults, error) {
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

// WatchServiceRelations returns a StringsWatcher, for each given
// service, that notifies of changes to the lifecycles of relations
// involving that service.
func (u *uniterBaseAPI) WatchServiceRelations(args params.Entities) (params.StringsWatchResults, error) {
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
func (u *uniterBaseAPI) CharmArchiveSha256(args params.CharmURLs) (params.StringResults, error) {
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

// CharmArchiveURLs returns the URLS for the charm archive
// (bundle) data for each charm url in the given parameters.
func (u *uniterBaseAPI) CharmArchiveURLs(args params.CharmURLs) (params.StringsResults, error) {
	apiHostPorts, err := u.st.APIHostPorts()
	if err != nil {
		return params.StringsResults{}, err
	}
	envUUID := u.st.EnvironUUID()
	result := params.StringsResults{
		Results: make([]params.StringsResult, len(args.URLs)),
	}
	for i, curl := range args.URLs {
		if _, err := charm.ParseURL(curl.URL); err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		urlPath := "/"
		if envUUID != "" {
			urlPath = path.Join(urlPath, "environment", envUUID)
		}
		urlPath = path.Join(urlPath, "charms")
		archiveURLs := make([]string, len(apiHostPorts))
		for j, server := range apiHostPorts {
			archiveURL := &url.URL{
				Scheme: "https",
				Host:   network.SelectInternalHostPort(server, false),
				Path:   urlPath,
			}
			q := archiveURL.Query()
			q.Set("url", curl.URL)
			q.Set("file", "*")
			archiveURL.RawQuery = q.Encode()
			archiveURLs[j] = archiveURL.String()
		}
		result.Results[i].Result = archiveURLs
	}
	return result, nil
}

// Relation returns information about all given relation/unit pairs,
// including their id, key and the local endpoint.
func (u *uniterBaseAPI) Relation(args params.RelationUnits) (params.RelationResults, error) {
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
func (u *uniterBaseAPI) Actions(args params.Entities) (params.ActionsQueryResults, error) {
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
		if action.Status() != state.ActionPending {
			results.Results[i].Error = common.ServerError(common.ErrActionNotAvailable)
			continue
		}
		results.Results[i].Action.Action = &params.Action{
			Name:       action.Name(),
			Parameters: action.Parameters(),
		}
	}

	return results, nil
}

// BeginActions marks the actions represented by the passed in Tags as running.
func (u *uniterBaseAPI) BeginActions(args params.Entities) (params.ErrorResults, error) {
	nothing := params.ErrorResults{}

	actionFn, err := u.authAndActionFromTagFn()
	if err != nil {
		return nothing, err
	}

	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Entities))}

	for i, arg := range args.Entities {
		action, err := actionFn(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		_, err = action.Begin()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return results, nil
}

// FinishActions saves the result of a completed Action
func (u *uniterBaseAPI) FinishActions(args params.ActionExecutionResults) (params.ErrorResults, error) {
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
		actionResults, err := paramsActionExecutionResultsToStateActionResults(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		_, err = action.Finish(actionResults)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}

	return results, nil
}

// paramsActionExecutionResultsToStateActionResults does exactly what
// the name implies.
func paramsActionExecutionResultsToStateActionResults(arg params.ActionExecutionResult) (state.ActionResults, error) {
	var status state.ActionStatus
	switch arg.Status {
	case params.ActionCancelled:
		status = state.ActionCancelled
	case params.ActionCompleted:
		status = state.ActionCompleted
	case params.ActionFailed:
		status = state.ActionFailed
	case params.ActionPending:
		status = state.ActionPending
	default:
		return state.ActionResults{}, errors.Errorf("unrecognized action status '%s'", arg.Status)
	}
	return state.ActionResults{
		Status:  status,
		Results: arg.Results,
		Message: arg.Message,
	}, nil
}

// RelationById returns information about all given relations,
// specified by their ids, including their key and the local
// endpoint.
func (u *uniterBaseAPI) RelationById(args params.RelationIds) (params.RelationResults, error) {
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

// JoinedRelations returns the tags of all relations for which each supplied unit
// has entered scope. It should be called RelationsInScope, but it's not convenient
// to make that change until we have versioned APIs.
func (u *uniterBaseAPI) JoinedRelations(args params.Entities) (params.StringsResults, error) {
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
func (u *uniterBaseAPI) CurrentEnvironUUID() (params.StringResult, error) {
	result := params.StringResult{}
	env, err := u.st.Environment()
	if err == nil {
		result.Result = env.UUID()
	}
	return result, err
}

// CurrentEnvironment returns the name and UUID for the current juju environment.
func (u *uniterBaseAPI) CurrentEnvironment() (params.EnvironmentResult, error) {
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
func (u *uniterBaseAPI) ProviderType() (params.StringResult, error) {
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
func (u *uniterBaseAPI) EnterScope(args params.RelationUnits) (params.ErrorResults, error) {
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
func (u *uniterBaseAPI) LeaveScope(args params.RelationUnits) (params.ErrorResults, error) {
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

// ReadSettings returns the local settings of each given set of
// relation/unit.
func (u *uniterBaseAPI) ReadSettings(args params.RelationUnits) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnits)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
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

// ReadRemoteSettings returns the remote settings of each given set of
// relation/local unit/remote unit.
func (u *uniterBaseAPI) ReadRemoteSettings(args params.RelationUnitPairs) (params.SettingsResults, error) {
	result := params.SettingsResults{
		Results: make([]params.SettingsResult, len(args.RelationUnitPairs)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.SettingsResults{}, err
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
func (u *uniterBaseAPI) UpdateSettings(args params.RelationUnitsSettings) (params.ErrorResults, error) {
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

// WatchRelationUnits returns a RelationUnitsWatcher for observing
// changes to every unit in the supplied relation that is visible to
// the supplied unit. See also state/watcher.go:RelationUnit.Watch().
func (u *uniterBaseAPI) WatchRelationUnits(args params.RelationUnits) (params.RelationUnitsWatchResults, error) {
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

// WatchAddresses returns a NotifyWatcher for observing changes
// to each unit's addresses.
func (u *uniterBaseAPI) WatchUnitAddresses(args params.Entities) (params.NotifyWatchResults, error) {
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

// GetMeterStatus returns meter status information for each unit.
func (u *uniterBaseAPI) GetMeterStatus(args params.Entities) (params.MeterStatusResults, error) {
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
		var status state.MeterStatus
		if canAccess(unitTag) {
			var unit *state.Unit
			unit, err = u.getUnit(unitTag)
			if err == nil {
				status, err = meterstatus.MeterStatusWrapper(unit.GetMeterStatus)
			}
			result.Results[i].Code = status.Code.String()
			result.Results[i].Info = status.Info
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *uniterBaseAPI) getUnit(tag names.UnitTag) (*state.Unit, error) {
	return u.st.Unit(tag.Id())
}

func (u *uniterBaseAPI) getService(tag names.ServiceTag) (*state.Service, error) {
	return u.st.Service(tag.Id())
}

func (u *uniterBaseAPI) getRelationUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.RelationUnit, error) {
	rel, unit, err := u.getRelationAndUnit(canAccess, relTag, unitTag)
	if err != nil {
		return nil, err
	}
	return rel.Unit(unit)
}

func (u *uniterBaseAPI) getOneRelationById(relId int) (params.RelationResult, error) {
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

func (u *uniterBaseAPI) getRelationAndUnit(canAccess common.AuthFunc, relTag string, unitTag names.UnitTag) (*state.Relation, *state.Unit, error) {
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

func (u *uniterBaseAPI) prepareRelationResult(rel *state.Relation, unit *state.Unit) (params.RelationResult, error) {
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
		Endpoint: multiwatcher.Endpoint{
			ServiceName: ep.ServiceName,
			Relation:    ep.Relation,
		},
	}, nil
}

func (u *uniterBaseAPI) getOneRelation(canAccess common.AuthFunc, relTag, unitTag string) (params.RelationResult, error) {
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

func (u *uniterBaseAPI) destroySubordinates(principal *state.Unit) error {
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

func (u *uniterBaseAPI) watchOneServiceRelations(tag names.ServiceTag) (params.StringsWatchResult, error) {
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

func (u *uniterBaseAPI) watchOneUnitConfigSettings(tag names.UnitTag) (string, error) {
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

func (u *uniterBaseAPI) watchOneUnitActionNotifications(tag names.UnitTag) (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	unit, err := u.getUnit(tag)
	if err != nil {
		return nothing, err
	}
	watch := unit.WatchActionNotifications()

	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: u.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

func (u *uniterBaseAPI) watchOneUnitAddresses(tag names.UnitTag) (string, error) {
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

func (u *uniterBaseAPI) watchOneRelationUnit(relUnit *state.RelationUnit) (params.RelationUnitsWatchResult, error) {
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

func (u *uniterBaseAPI) watchOneUnitMeterStatus(tag names.UnitTag) (string, error) {
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

func (u *uniterBaseAPI) checkRemoteUnit(relUnit *state.RelationUnit, remoteUnitTag string) (string, error) {
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
	remoteServiceName, err := names.UnitService(remoteUnitName)
	if err != nil {
		return "", common.ErrPerm
	}
	rel := relUnit.Relation()
	_, err = rel.RelatedEndpoints(remoteServiceName)
	if err != nil {
		return "", common.ErrPerm
	}
	return remoteUnitName, nil
}

// authAndActionFromTagFn first authenticates the request, and then returns
// a function with which to authenticate and retrieve each action in the
// request.
func (u *uniterBaseAPI) authAndActionFromTagFn() (func(string) (*state.Action, error), error) {
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
		action, err := u.st.ActionByTag(actionTag)
		if err != nil {
			return nil, err
		}
		receiverTag, err := names.ActionReceiverTag(action.Receiver())
		if err != nil {
			return nil, err
		}
		if unit != receiverTag {
			return nil, common.ErrPerm
		}

		if !canAccess(receiverTag) {
			return nil, common.ErrPerm
		}
		return action, nil
	}, nil
}

func convertRelationSettings(settings map[string]interface{}) (params.Settings, error) {
	result := make(params.Settings)
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

func leadershipSettingsAccessorFactory(
	st *state.State,
	resources *common.Resources,
	auth common.Authorizer,
) *leadershipapiserver.LeadershipSettingsAccessor {
	registerWatcher := func(serviceId string) (string, error) {
		service, err := st.Service(serviceId)
		if err != nil {
			return "", err
		}
		w := service.WatchLeaderSettings()
		if _, ok := <-w.Changes(); ok {
			return resources.Register(w), nil
		}
		return "", watcher.EnsureErr(w)
	}
	getSettings := func(serviceId string) (map[string]string, error) {
		service, err := st.Service(serviceId)
		if err != nil {
			return nil, err
		}
		return service.LeaderSettings()
	}
	writeSettings := func(token leadership.Token, serviceId string, settings map[string]string) error {
		service, err := st.Service(serviceId)
		if err != nil {
			return err
		}
		return service.UpdateLeaderSettings(token, settings)
	}
	return leadershipapiserver.NewLeadershipSettingsAccessor(
		auth,
		registerWatcher,
		getSettings,
		st.LeadershipChecker().LeadershipCheck,
		writeSettings,
	)
}
