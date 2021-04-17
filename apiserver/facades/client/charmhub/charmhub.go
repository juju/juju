// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/charm/v8"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/environs/config"
)

const (
	// TimeoutDuration represents how long we should wait before a response back
	// from the API before timing out.
	TimeoutDuration = time.Second * 30
)

var logger = loggo.GetLogger("juju.apiserver.charmhub")

// Backend defines the state methods this facade needs, so they can be
// mocked for testing.
type Backend interface {
	ModelConfig() (*config.Config, error)
}

// ClientFactory defines a factory for creating clients from a given url.
type ClientFactory interface {
	Client(string) (Client, error)
}

// Client represents a CharmHub Client for making queries to the CharmHub API.
type Client interface {
	URL() string
	Info(ctx context.Context, name string, options ...charmhub.InfoOption) (transport.InfoResponse, error)
	Find(ctx context.Context, query string, options ...charmhub.FindOption) ([]transport.FindResponse, error)
}

// CharmHubAPI API provides the CharmHub API facade for version 1.
type CharmHubAPI struct {
	backend Backend
	auth    facade.Authorizer
	client  Client
}

// NewFacade creates a new CharmHubAPI facade.
func NewFacade(ctx facade.Context) (*CharmHubAPI, error) {
	m, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newCharmHubAPI(m, ctx.Auth(), charmHubClientFactory{})
}

func newCharmHubAPI(backend Backend, authorizer facade.Authorizer, clientFactory ClientFactory) (*CharmHubAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	modelCfg, err := backend.ModelConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	url, _ := modelCfg.CharmHubURL()
	client, err := clientFactory.Client(url)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &CharmHubAPI{
		auth:   authorizer,
		client: client,
	}, nil
}

// Info queries the CharmHub API with a given entity ID.
func (api *CharmHubAPI) Info(ctx context.Context, arg params.Info) (params.CharmHubEntityInfoResult, error) {
	logger.Tracef("Info(%v)", arg.Tag)

	tag, err := names.ParseApplicationTag(arg.Tag)
	if err != nil {
		return params.CharmHubEntityInfoResult{}, errors.BadRequestf("tag value is empty")
	}

	var options []charmhub.InfoOption
	if arg.Channel != "" {
		ch, err := charm.ParseChannelNormalize(arg.Channel)
		if err != nil {
			return params.CharmHubEntityInfoResult{}, errors.BadRequestf("channel %q is invalid", arg.Channel)
		}
		options = append(options, charmhub.WithInfoChannel(ch.String()))
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, TimeoutDuration)
	defer cancel()

	info, err := api.client.Info(ctx, tag.Id(), options...)
	if err != nil {
		return params.CharmHubEntityInfoResult{}, errors.Trace(err)
	}
	result, err := convertCharmInfoResult(info)
	return params.CharmHubEntityInfoResult{Result: result}, err
}

// Find queries the CharmHub API with a given entity ID.
func (api *CharmHubAPI) Find(ctx context.Context, arg params.Query) (params.CharmHubEntityFindResult, error) {
	logger.Tracef("Find(%v)", arg.Query)

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, TimeoutDuration)
	defer cancel()

	results, err := api.client.Find(ctx, arg.Query, populateFindOptions(arg)...)
	if err != nil {
		return params.CharmHubEntityFindResult{}, errors.Trace(err)
	}
	return params.CharmHubEntityFindResult{Results: convertCharmFindResults(results)}, nil
}

type charmHubClientFactory struct{}

func (charmHubClientFactory) Client(url string) (Client, error) {
	cfg, err := charmhub.CharmHubConfigFromURL(url, logger.Child("client"))
	if err != nil {
		return nil, errors.Trace(err)
	}
	client, err := charmhub.NewClient(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

func populateFindOptions(arg params.Query) []charmhub.FindOption {
	var options []charmhub.FindOption

	if arg.Category != "" {
		options = append(options, charmhub.WithFindCategory(arg.Category))
	}
	if arg.Channel != "" {
		options = append(options, charmhub.WithFindChannel(arg.Channel))
	}
	if arg.CharmType != "" {
		options = append(options, charmhub.WithFindType(arg.CharmType))
	}
	if arg.Platforms != "" {
		options = append(options, charmhub.WithFindPlatforms(arg.Platforms))
	}
	if arg.Publisher != "" {
		options = append(options, charmhub.WithFindPublisher(arg.Publisher))
	}
	if arg.RelationRequires != "" {
		options = append(options, charmhub.WithFindRelationRequires(arg.RelationRequires))
	}
	if arg.RelationProvides != "" {
		options = append(options, charmhub.WithFindRelationProvides(arg.RelationProvides))
	}

	return options
}
