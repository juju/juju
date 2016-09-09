// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewConfigCommandForTest returns a SetCommand with the api provided as specified.
func NewConfigCommandForTest(api configCommandAPI) cmd.Command {
	return modelcmd.Wrap(&configCommand{
		api: api,
	})
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommandForTest(api serviceAddUnitAPI) cmd.Command {
	return modelcmd.Wrap(&addUnitCommand{
		api: api,
	})
}

type Patcher interface {
	PatchValue(dest, value interface{})
}

func PatchNewCharmStoreClient(s Patcher, url string) {
	s.PatchValue(&newCharmStoreClient, func(bakeryClient *httpbakery.Client) *csclient.Client {
		return csclient.New(csclient.Params{
			URL:          url,
			BakeryClient: bakeryClient,
		})
	})
}
