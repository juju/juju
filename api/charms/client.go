// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the charms API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the charms API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Charms")
	return &Client{ClientFacade: frontend, facade: backend}
}

// CharmInfo holds information about a charm.
type CharmInfo struct {
	Revision int
	URL      string
	Config   *charm.Config
	Meta     *charm.Meta
	Actions  *charm.Actions
}

// CharmInfo returns information about the requested charm.
func (c *Client) CharmInfo(charmURL string) (*CharmInfo, error) {
	args := params.CharmInfo{CharmURL: charmURL}
	info := &CharmInfo{}
	if err := c.facade.FacadeCall("CharmInfo", args, info); err != nil {
		return nil, err
	}
	return info, nil
}

// List returns a list of charm URLs currently in the state.
// If supplied parameter contains any names, the result will be filtered
// to return only the charms with supplied names.
func (c *Client) List(names []string) ([]string, error) {
	charms := &params.CharmsListResult{}
	args := params.CharmsList{Names: names}
	if err := c.facade.FacadeCall("List", args, charms); err != nil {
		return nil, errors.Trace(err)
	}
	return charms.CharmURLs, nil
}

// IsMetered returns whether or not the charm is metered.
func (c *Client) IsMetered(charmURL string) (bool, error) {
	args := params.CharmInfo{CharmURL: charmURL}
	metered := &params.IsMeteredResult{}
	if err := c.facade.FacadeCall("IsMetered", args, metered); err != nil {
		return false, err
	}
	return metered.Metered, nil
}
