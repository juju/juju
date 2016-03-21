// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Charms", 2, NewAPI)
}

var getState = func(st *state.State) charmsAccess {
	return stateShim{st}
}

// Charms defines the methods on the charms API end point.
type Charms interface {
	List(args params.CharmsList) (params.CharmsListResult, error)
	CharmInfo(CharmInfo) (params.CharmInfo, error)
	IsMetered(CharmInfo) (bool, error)
}

// API implements the charms interface and is the concrete
// implementation of the api end point.
type API struct {
	access     charmsAccess
	authorizer common.Authorizer
}

// NewAPI returns a new charms API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		access:     getState(st),
		authorizer: authorizer,
	}, nil
}

// CharmInfo holds the url of the requested charm.
type CharmInfo struct {
	CharmURL string
}

// CharmInfo returns information about the requested charm.
func (a *API) CharmInfo(args CharmInfo) (params.CharmInfo, error) {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return params.CharmInfo{}, err
	}
	aCharm, err := a.access.Charm(curl)
	if err != nil {
		return params.CharmInfo{}, err
	}
	info := params.CharmInfo{
		Revision: aCharm.Revision(),
		URL:      curl.String(),
		Config:   aCharm.Config(),
		Meta:     aCharm.Meta(),
		Actions:  aCharm.Actions(),
	}
	return info, nil
}

// List returns a list of charm URLs currently in the state.
// If supplied parameter contains any names, the result will be filtered
// to return only the charms with supplied names.
func (a *API) List(args params.CharmsList) (params.CharmsListResult, error) {
	charms, err := a.access.AllCharms()
	if err != nil {
		return params.CharmsListResult{}, errors.Annotatef(err, " listing charms ")
	}

	names := set.NewStrings(args.Names...)
	checkName := !names.IsEmpty()
	charmURLs := []string{}
	for _, aCharm := range charms {
		charmURL := aCharm.URL()
		if checkName {
			if !names.Contains(charmURL.Name) {
				continue
			}
		}
		charmURLs = append(charmURLs, charmURL.String())
	}
	return params.CharmsListResult{CharmURLs: charmURLs}, nil
}

// IsMetered returns whether or not the charm is metered.
func (a *API) IsMetered(args CharmInfo) (params.IsMeteredResult, error) {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return params.IsMeteredResult{false}, err
	}
	aCharm, err := a.access.Charm(curl)
	if err != nil {
		return params.IsMeteredResult{false}, err
	}
	if aCharm.Metrics() != nil && len(aCharm.Metrics().Metrics) > 0 {
		return params.IsMeteredResult{true}, nil
	}
	return params.IsMeteredResult{false}, nil
}
