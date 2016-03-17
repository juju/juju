// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/modelcmd"
)

// TODO(ericsnow) Factor out code from cmd/juju/commands/common.go and                                                                                                                                                                                                          │
// cmd/envcmd/base.go into cmd/charmstore.go and cmd/apicontext.go. Then                                                                                                                                                                                                        │
// use those here instead of copy-and-pasting here.

///////////////////
// The charmstoreSpec code is based loosely on code in cmd/juju/commands/deploy.go.

// CharmstoreSpec provides the functionality needed to open a charm
// store client.
type CharmstoreSpec interface {
	// Connect connects to the specified charm store.
	Connect(ctx *cmd.Context) (*charmstore.Client, error)
}

type charmstoreSpec struct{}

// newCharmstoreSpec creates a new charm store spec with default
// settings.
func newCharmstoreSpec() CharmstoreSpec {
	return &charmstoreSpec{}
}

// Connect implements CharmstoreSpec.
func (cs charmstoreSpec) Connect(ctx *cmd.Context) (*charmstore.Client, error) {
	// Note that creating the API context in Connect is technically
	// wrong, as it means we'll be creating the bakery context
	// (and reading/writing the cookies) each time it's called.
	// TODO(ericsnow) Move apiContext to a field on charmstoreSpec.
	apiContext, err := modelcmd.NewAPIContext(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We use the default for URL. We do not bother with VisitWebPage
	// since that is addressed via BakeryClient.
	config := charmstore.ClientConfig{
		NewCharmStoreParams: charmrepo.NewCharmStoreParams{
			BakeryClient: apiContext.BakeryClient,
		},
	}
	client := charmstore.NewClient(config)
	client.Closer = apiContext

	return client, nil
}
