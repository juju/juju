// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

type LXDProfileBackendV2 interface {
	Charm(*charm.URL) (LXDProfileCharmV2, error)
	Machine(string) (LXDProfileMachineV2, error)
	Unit(string) (LXDProfileUnitV2, error)
}

// LXDProfileMachineV2 describes machine-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileMachineV2 interface {
	CharmProfiles() ([]string, error)
	WatchInstanceData() state.NotifyWatcher
}

// LXDProfileUnitV2 describes unit-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileUnitV2 interface {
	ApplicationName() string
	AssignedMachineId() (string, error)
	CharmURL() (*charm.URL, bool)
	Name() string
	Tag() names.Tag
}

// LXDProfileCharmV2 describes charm-receiver state methods
// for executing a lxd profile upgrade.
type LXDProfileCharmV2 interface {
	LXDProfile() lxdprofile.Profile
}

type LXDProfileAPIv2 struct {
	backend   LXDProfileBackendV2
	resources facade.Resources

	logger     loggo.Logger
	accessUnit common.GetAuthFunc
}

// LXDProfileAPIv2 returns a new LXDProfileAPIv2. Currently both
// GetAuthFuncs can used to determine current permissions.
func NewLXDProfileAPIv2(
	backend LXDProfileBackendV2,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileAPIv2 {
	logger.Tracef("LXDProfileAPIv2 called with %s", authorizer.GetAuthTag())
	return &LXDProfileAPIv2{
		backend:    backend,
		resources:  resources,
		accessUnit: accessUnit,
		logger:     logger,
	}
}

// LXDProfileStateV2 implements the LXDProfileBackendV2 indirection
// over state.State.
type LXDProfileStateV2 struct {
	st *state.State
}

func (s LXDProfileStateV2) Machine(id string) (LXDProfileMachineV2, error) {
	m, err := s.st.Machine(id)
	return &lxdProfileMachineV2{m}, err
}

func (s LXDProfileStateV2) Unit(id string) (LXDProfileUnitV2, error) {
	return s.st.Unit(id)
}

func (s LXDProfileStateV2) Charm(curl *charm.URL) (LXDProfileCharmV2, error) {
	c, err := s.st.Charm(curl)
	return &lxdProfileCharmV2{c}, err
}

type lxdProfileMachineV2 struct {
	*state.Machine
}

type lxdProfileCharmV2 struct {
	*state.Charm
}

func (c *lxdProfileCharmV2) LXDProfile() lxdprofile.Profile {
	profile := c.Charm.LXDProfile()
	if profile == nil {
		return lxdprofile.Profile{}
	}
	return lxdprofile.Profile{
		Config:      profile.Config,
		Description: profile.Description,
		Devices:     profile.Devices,
	}
}

// ExternalLXDProfileAPIv2 can be used for API registration.
func NewExternalLXDProfileAPIv2(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
	accessUnit common.GetAuthFunc,
	logger loggo.Logger,
) *LXDProfileAPIv2 {
	return NewLXDProfileAPIv2(
		LXDProfileStateV2{st},
		resources,
		authorizer,
		accessUnit,
		logger,
	)
}

// WatchInstanceData returns a NotifyWatcher for observing
// changes to the lxd profile for one unit.
func (u *LXDProfileAPIv2) WatchInstanceData(args params.Entities) (params.NotifyWatchResults, error) {
	u.logger.Tracef("Starting WatchInstanceData with %+v", args)
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		u.logger.Tracef("WatchInstanceData error %+v", err)
		return params.NotifyWatchResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		machine, err := u.getLXDProfileMachineV2(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		watcherId, err := u.watchOneInstanceData(machine)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].NotifyWatcherId = watcherId

	}
	u.logger.Tracef("WatchInstanceData returning %+v", result)
	return result, nil
}

func (u *LXDProfileAPIv2) watchOneInstanceData(machine LXDProfileMachineV2) (string, error) {
	watch := machine.WatchInstanceData()
	if _, ok := <-watch.Changes(); ok {
		return u.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// LXDProfileName returns the name of the lxd profile applied to the unit's
// machine for the current charm version.
func (u *LXDProfileAPIv2) LXDProfileName(args params.Entities) (params.StringResults, error) {
	u.logger.Tracef("Starting LXDProfileName with %+v", args)
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		return params.StringResults{}, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, machine, err := u.getLXDProfileUnitMachineV2(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		name, err := u.getOneLXDProfileName(unit, machine)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].Result = name

	}
	return result, nil
}

func (u *LXDProfileAPIv2) getOneLXDProfileName(unit LXDProfileUnitV2, machine LXDProfileMachineV2) (string, error) {
	profileNames, err := machine.CharmProfiles()
	if err != nil {
		return "", err
	}
	appName := unit.ApplicationName()
	return lxdprofile.MatchProfileNameByAppName(profileNames, appName)
}

// LXDProfileRequired returns true if charm has an lxd profile in it.
func (u *LXDProfileAPIv2) LXDProfileRequired(args params.CharmURLs) (params.BoolResults, error) {
	u.logger.Tracef("Starting LXDProfileRequired with %+v", args)
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.URLs)),
	}
	for i, arg := range args.URLs {
		curl, err := charm.ParseURL(arg.URL)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		required, err := u.getOneLXDProfileRequired(curl)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		result.Results[i].Result = required
	}
	return result, nil
}

func (u *LXDProfileAPIv2) getOneLXDProfileRequired(curl *charm.URL) (bool, error) {
	ch, err := u.backend.Charm(curl)
	if err != nil {
		return false, err
	}
	return !ch.LXDProfile().Empty(), nil
}

func (u *LXDProfileAPIv2) getLXDProfileMachineV2(tag names.Tag) (LXDProfileMachineV2, error) {
	_, machine, err := u.getLXDProfileUnitMachineV2(tag)
	return machine, err
}

func (u *LXDProfileAPIv2) getLXDProfileUnitMachineV2(tag names.Tag) (LXDProfileUnitV2, LXDProfileMachineV2, error) {
	var id string
	if tag.Kind() != names.UnitTagKind {
		return nil, nil, errors.Errorf("not a unit tag")
	}
	unit, err := u.backend.Unit(tag.Id())
	if err != nil {
		return nil, nil, err
	}
	id, err = unit.AssignedMachineId()
	if err != nil {
		return nil, nil, err
	}
	machine, err := u.backend.Machine(id)
	return unit, machine, err
}
