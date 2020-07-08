// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
)

var logger = loggo.GetLogger("juju.apiserver.charmhub")

// Client represents a charmhub Client for making queries to the charmhub API.
type Client interface {
	Info(ctx context.Context, name string) (transport.InfoResponse, error)
	Find(ctx context.Context, query string) ([]transport.FindResponse, error)
}

// CharmHubAPI API provides the charmhub API facade for version 1.
type CharmHubAPI struct {
	auth facade.Authorizer

	// newClientFunc is for testing purposes to facilitate using mocks.
	client Client
}

// NewFacade creates a new CharmHubAPI facade.
func NewFacade(ctx facade.Context) (*CharmHubAPI, error) {
	auth := ctx.Auth()
	client, err := charmhub.NewClient(charmhub.CharmhubConfig())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCharmHubAPI(auth, client)
}

func newCharmHubAPI(authorizer facade.Authorizer, client Client) (*CharmHubAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &CharmHubAPI{auth: authorizer, client: client}, nil
}

// Info queries the charmhub API with a given entity ID.
func (api *CharmHubAPI) Info(arg params.Entity) (params.CharmHubEntityInfoResult, error) {
	logger.Tracef("Info(%v)", arg.Tag)

	tag, err := names.ParseApplicationTag(arg.Tag)
	if err != nil {
		return params.CharmHubEntityInfoResult{}, errors.BadRequestf("arg value is empty")
	}
	// TODO (stickupkid): Create a proper context to be used here.
	info, err := api.client.Info(context.TODO(), tag.Id())
	if err != nil {
		return params.CharmHubEntityInfoResult{}, errors.Trace(err)
	}
	return params.CharmHubEntityInfoResult{Result: convertCharmInfoResult(info)}, nil
}

// Find queries the charmhub API with a given entity ID.
func (api *CharmHubAPI) Find(arg params.Query) (params.CharmHubEntityFindResult, error) {
	logger.Tracef("Find(%v)", arg.Query)

	// TODO (stickupkid): Create a proper context to be used here.
	results, err := api.client.Find(context.TODO(), arg.Query)
	if err != nil {
		return params.CharmHubEntityFindResult{}, errors.Trace(err)
	}
	return params.CharmHubEntityFindResult{Results: convertCharmFindResults(results)}, nil
}
