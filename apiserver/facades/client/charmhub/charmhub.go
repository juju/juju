// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	commonerrors "github.com/juju/juju/apiserver/common/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
)

var logger = loggo.GetLogger("juju.apiserver.charmhub")

type Client interface {
	Info(ctx context.Context, name string) (transport.InfoResponse, error)
}

// API provides the charmhub API facade for version 1.
type CharmHubAPI struct {
	auth facade.Authorizer

	// newClientFunc is for testing purposes to facilitate using mocks.
	newClientFunc func(charmhub.Config) (Client, error)
}

func NewFacade(ctx facade.Context) (*CharmHubAPI, error) {
	auth := ctx.Auth()
	newClientFunc := func(p charmhub.Config) (Client, error) {
		return charmhub.NewClient(p)
	}
	return newCharmHubAPI(auth, newClientFunc)
}

func newCharmHubAPI(authorizer facade.Authorizer, newClientFunc func(charmhub.Config) (Client, error)) (*CharmHubAPI, error) {
	if !authorizer.AuthClient() {
		return nil, commonerrors.ErrPerm
	}
	return &CharmHubAPI{auth: authorizer, newClientFunc: newClientFunc}, nil
}

func (api *CharmHubAPI) Info(arg params.Entity) (params.CharmHubCharmInfoResult, error) {
	logger.Tracef("Info()")
	tag, err := names.ParseApplicationTag(arg.Tag)
	if err != nil {
		return params.CharmHubCharmInfoResult{}, errors.BadRequestf("arg value is empty")
	}
	// TODO: (hml) 2020-06-17
	// Add model config value for charmhub-url to charmhub client New().
	// once implemented.
	// TODO: (hml) 2020-06-19
	// PR Comment
	// "I think it's fair to say we should cache this, as generating
	// a new client for every info is wasteful. Lets tackle at a later point."
	chClient, err := api.newClientFunc(charmhub.CharmhubConfig())
	if err != nil {
		return params.CharmHubCharmInfoResult{}, errors.Annotate(err, "could not get charm hub client")
	}
	// TODO:
	// Create a proper context to be used here.
	info, err := chClient.Info(context.TODO(), tag.Id())
	return params.CharmHubCharmInfoResult{Result: convertCharmInfoResult(info)}, err
}
