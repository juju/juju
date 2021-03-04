// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const charmHubFacade = "CharmHub"

// Client allows access to the CharmHub API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the CharmHub API.
func NewClient(callCloser base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(callCloser, charmHubFacade)
	return newClientFromFacade(frontend, backend)
}

// NewClientFromFacade creates a new charmHub client using the input
// client facade and facade caller.
func newClientFromFacade(frontend base.ClientFacade, backend base.FacadeCaller) *Client {
	return &Client{
		ClientFacade: frontend,
		facade:       backend,
	}
}

// Info queries the CharmHub API for information for a given name.
func (c *Client) Info(name string, options ...InfoOption) (InfoResponse, error) {
	opts := newInfoOptions()
	for _, option := range options {
		option(opts)
	}

	args := params.Info{
		Tag:     names.NewApplicationTag(name).String(),
		Channel: opts.channel,
	}
	var result params.CharmHubEntityInfoResult
	if err := c.facade.FacadeCall("Info", args, &result); err != nil {
		return InfoResponse{}, errors.Trace(err)
	}

	return convertCharmInfoResult(result.Result), nil
}

// Find queries the CharmHub API finding potential charms or bundles for the
// given query.
func (c *Client) Find(query string, options ...FindOption) ([]FindResponse, error) {
	opts := newFindOptions()
	for _, option := range options {
		option(opts)
	}

	args := params.Query{
		Query:            query,
		Category:         opts.category,
		Channel:          opts.channel,
		CharmType:        opts.charmType,
		Platforms:        opts.platforms,
		Publisher:        opts.publisher,
		RelationRequires: opts.relationRequires,
		RelationProvides: opts.relationProvides,
	}

	var result params.CharmHubEntityFindResult
	if err := c.facade.FacadeCall("Find", args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return convertCharmFindResults(result.Results), nil
}

// InfoOption to be passed to Info to customize the resulting request.
type InfoOption func(*infoOptions)

type infoOptions struct {
	channel string
}

// WithInfoChannel sets the channel on the option.
func WithInfoChannel(ch string) InfoOption {
	return func(infoOptions *infoOptions) {
		infoOptions.channel = ch
	}
}

// Create a infoOptions instance with default values.
func newInfoOptions() *infoOptions {
	return &infoOptions{}
}

// FindOption to be passed to Find to customize the resulting request.
type FindOption func(*findOptions)

type findOptions struct {
	category         string
	channel          string
	charmType        string
	platforms        string
	publisher        string
	relationRequires string
	relationProvides string
}

// WithFindCategory sets the category on the option.
func WithFindCategory(category string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.category = category
	}
}

// WithFindChannel sets the channel on the option.
func WithFindChannel(channel string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.channel = channel
	}
}

// WithFindType sets the charmType on the option.
func WithFindType(charmType string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.charmType = charmType
	}
}

// WithFindPlatforms sets the charmPlatforms on the option.
func WithFindPlatforms(platforms string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.platforms = platforms
	}
}

// WithFindPublisher sets the publisher on the option.
func WithFindPublisher(publisher string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.publisher = publisher
	}
}

// WithFindRelationRequires sets the relationRequires on the option.
func WithFindRelationRequires(relationRequires string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.relationRequires = relationRequires
	}
}

// WithFindRelationProvides sets the relationProvides on the option.
func WithFindRelationProvides(relationProvides string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.relationProvides = relationProvides
	}
}

// Create a findOptions instance with default values.
func newFindOptions() *findOptions {
	return &findOptions{}
}
