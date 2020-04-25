// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package romulus

import (
	"github.com/juju/charm/v7"
	"github.com/juju/charmrepo/v5"
	"github.com/juju/errors"
	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
)

// CharmResolver interface defines the functionality to resolve a charm URL.
type CharmResolver interface {
	// Resolve resolves the charm URL.
	Resolve(client *httpbakery.Client, charmURL string) (string, error)
}

// CharmStoreResolver implements the CharmResolver interface.
type CharmStoreResolver struct {
	csURL string
}

// NewCharmStoreResolverForControllerCmd creates a new charm store resolver
// that connects to the controller configured charmstore-url.
var NewCharmStoreResolverForControllerCmd = newCharmStoreResolverForControllerCmdImpl

func newCharmStoreResolverForControllerCmdImpl(c *modelcmd.ControllerCommandBase) (CharmResolver, error) {
	controllerAPIRoot, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerAPI := controller.NewClient(controllerAPIRoot)
	controllerCfg, err := controllerAPI.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CharmStoreResolver{
		csURL: controllerCfg.CharmStoreURL(),
	}, nil
}

// Resolve implements the CharmResolver interface.
func (r *CharmStoreResolver) Resolve(client *httpbakery.Client, charmURL string) (string, error) {
	repo := charmrepo.NewCharmStore(charmrepo.NewCharmStoreParams{
		BakeryClient: client,
		URL:          r.csURL,
	})

	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return "", errors.Annotate(err, "could not parse charm url")
	}
	// ignore local charm urls
	if curl.Schema == "local" {
		return charmURL, nil
	}
	resolvedURL, _, err := repo.Resolve(curl)
	if err != nil {
		return "", errors.Trace(err)
	}
	return resolvedURL.String(), nil
}
