// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/controller"
	corecharm "github.com/juju/juju/core/charm"
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
	return charmstore.NewCachingClient(state.MacaroonCache{st}, controllerCfg.CharmStoreURL())
}

type csClient struct {
	client charmstore.Client
}

func (cs *csClient) GetResource(req repositories.ResourceRequest) (charmstore.ResourceData, error) {
	stChannel := req.CharmID.Origin.Channel
	if stChannel == nil {
		return charmstore.ResourceData{}, errors.Errorf("Missing channel for %q", req.CharmID.URL.Name)
	}
	channel, err := corecharm.MakeChannel(stChannel.Track, stChannel.Risk, stChannel.Branch)
	if err != nil {
		return charmstore.ResourceData{}, errors.Trace(err)
	}
	csReq := charmstore.ResourceRequest{
		Charm:    req.CharmID.URL,
		Channel:  csparams.Channel(channel.String()),
		Name:     req.Name,
		Revision: req.Revision,
	}
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
