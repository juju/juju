// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmcmd

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"

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
	Connect(ctx *cmd.Context) (charmstore.Client, io.Closer, error)
}

type charmstoreSpec struct{}

// newCharmstoreSpec creates a new charm store spec with default
// settings.
func newCharmstoreSpec() CharmstoreSpec {
	return charmstoreSpec{}
}

// Connect implements CharmstoreSpec.
func (cs charmstoreSpec) Connect(ctx *cmd.Context) (charmstore.Client, io.Closer, error) {
	// Note that creating the API context in Connect is technically
	// wrong, as it means we'll be creating the bakery context
	// (and reading/writing the cookies) each time it's called.
	// TODO(ericsnow) Move apiContext to a field on charmstoreSpec.
	apiContext, err := modelcmd.NewAPIContext(ctx)
	if err != nil {
		return charmstore.Client{}, nil, errors.Trace(err)
	}
	// We use the default for URL.
	client := charmstore.NewClient(charmstore.ClientConfig{
		BakeryClient: apiContext.BakeryClient,
	})

	return client, apiContext, nil
}
