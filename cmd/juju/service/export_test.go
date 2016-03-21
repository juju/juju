// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/cmd"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewSetCommandForTest returns a SetCommand with the api provided as specified.
func NewSetCommandForTest(serviceAPI serviceAPI) cmd.Command {
	return modelcmd.Wrap(&setCommand{
		serviceApi: serviceAPI,
	})
}

// NewGetCommand returns a GetCommand with the api provided as specified.
func NewGetCommandForTest(api getServiceAPI) cmd.Command {
	return modelcmd.Wrap(&getCommand{
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
	original := newCharmStoreClient
	s.PatchValue(&newCharmStoreClient, func(bakeryClient *httpbakery.Client) *csClient {
		csclient := original(bakeryClient)
		csclient.params.URL = url
		return csclient
	})
}
