// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/charm/v9"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/errors"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/resource/repositories"
	"github.com/juju/juju/state"
)

type charmStoreOpener struct {
	st csClientState
}

func newCharmStoreOpener(st csClientState) *charmStoreOpener {
	return &charmStoreOpener{st}
}

type csClientState interface {
	Charm(*charm.URL) (*state.Charm, error)
	ControllerConfig() (controller.Config, error)
}

func newCharmStoreClient(st csClientState) (charmstore.Client, error) {
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return charmstore.Client{}, errors.Trace(err)
	}
	return charmstore.NewCachingClient(state.MacaroonCache{
		MacaroonCacheState: st,
	}, controllerCfg.CharmStoreURL())
}

type csClient struct {
	client charmstore.Client
}

func (cs *csClient) GetResource(req repositories.ResourceRequest) (charmstore.ResourceData, error) {
	csReq := charmstore.ResourceRequest{
		Charm:    req.CharmID.URL,
		Name:     req.Name,
		Revision: req.Revision,
	}

	// CharmStore charms may or may not have a channel, thus
	// an empty string is valid for the request channel.  It
	// will be handled by the charmstore client.
	stChannel := req.CharmID.Origin.Channel
	if stChannel == nil {
		return cs.client.GetResource(csReq)
	}

	channel, err := charm.MakeChannel(stChannel.Track, stChannel.Risk, stChannel.Branch)
	if err != nil {
		return charmstore.ResourceData{}, errors.Trace(err)
	}
	csReq.Channel = csparams.Channel(channel.String())
	return cs.client.GetResource(csReq)
}

// NewClient opens a new charm store client.
func (cs *charmStoreOpener) NewClient() (*ResourceRetryClient, error) {
	// TODO(ericsnow) Use a valid charm store client.
	client, err := newCharmStoreClient(cs.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newRetryClient(&csClient{
		client,
	}), nil
}
