// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"context"
	"net/url"

	"github.com/juju/errors"
	"github.com/kr/pretty"

	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/state"
)

type charmHubOpener struct {
	st chClientState
}

func newCharmHubOpener(st chClientState) *charmHubOpener {
	return &charmHubOpener{st}
}

func (ch *charmHubOpener) NewClient() (*ResourceRetryClient, error) {
	client, err := newCharmHubClient(ch.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newRetryClient(client), nil
}

// chClientState represents a state which can provide a model to create a
// CharmHub client.
type chClientState interface {
	Model() (*state.Model, error)
}

func newCharmHubClient(st chClientState) (ResourceGetter, error) {
	m, err := st.Model()
	if err != nil {
		return &CharmHubClient{}, errors.Trace(err)
	}
	modelCfg, err := m.Config()
	if err != nil {
		return &CharmHubClient{}, errors.Trace(err)
	}

	var chCfg charmhub.Config
	chURL, ok := modelCfg.CharmHubURL()
	if ok {
		chCfg, err = charmhub.CharmHubConfigFromURL(chURL, logger)
	} else {
		chCfg, err = charmhub.CharmHubConfig(logger)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	chClient, err := charmhub.NewClient(chCfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CharmHubClient{client: chClient, logger: logger.ChildWithLabels("charmhub", corelogger.CHARMHUB)}, nil
}

type CharmHubClient struct {
	client CharmHub
	logger Logger
}

// GetResource returns data about the resource including an io.ReadCloser
// to download the resource.  The caller is responsible for closing it.
func (ch *CharmHubClient) GetResource(req ResourceRequest) (ResourceData, error) {
	ch.logger.Tracef("GetResource(%s)", pretty.Sprint(req))
	var data ResourceData

	origin := req.CharmID.Origin

	stChannel := origin.Channel
	if stChannel == nil {
		return data, errors.Errorf("missing channel for %q", req.CharmID.URL.Name)
	}
	channel, err := charm.MakeChannel(stChannel.Track, stChannel.Risk, stChannel.Branch)
	if err != nil {
		return data, errors.Trace(err)
	}

	if req.CharmID.URL == nil {
		return data, errors.Errorf("missing charm url for resource %q", req.Name)
	}

	cfg, err := charmhub.DownloadOneFromChannel(origin.ID, channel.String(), charmhub.RefreshBase{
		Architecture: origin.Platform.Architecture,
		Name:         origin.Platform.OS,
		Channel:      origin.Platform.Series,
	})
	if err != nil {
		return data, errors.Trace(err)
	}

	refreshResp, err := ch.client.Refresh(context.TODO(), cfg)
	if err != nil {
		return data, errors.Trace(err)
	}
	if len(refreshResp) == 0 {
		return data, errors.Errorf("no download refresh responses received")
	}
	resp := refreshResp[0]
	r, resourceURL, err := resourceFromRevision(req.Name, resp.Entity.Resources)
	if err != nil {
		return data, errors.Trace(err)
	}
	data.Resource = r

	ch.logger.Tracef("Read resource %q from %q", r.Name, resourceURL)

	data.ReadCloser, err = ch.client.DownloadResource(context.TODO(), resourceURL)
	if err != nil {
		return data, errors.Trace(err)
	}
	return data, nil
}

func resourceFromRevision(name string, revs []transport.ResourceRevision) (charmresource.Resource, *url.URL, error) {
	var rev transport.ResourceRevision
	for _, v := range revs {
		if v.Name == name {
			rev = v
		}
	}
	if rev.Name != name {
		return charmresource.Resource{}, nil, errors.Trace(errors.NotFoundf("resource %q", name))
	}
	resType, err := charmresource.ParseType(rev.Type)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}
	fingerprint, err := charmresource.ParseFingerprint(rev.Download.HashSHA384)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}

	r := charmresource.Resource{
		Fingerprint: fingerprint,
		Meta: charmresource.Meta{
			Name:        rev.Name,
			Type:        resType,
			Path:        rev.Filename,
			Description: rev.Description,
		},
		Origin:   charmresource.OriginStore,
		Revision: rev.Revision,
		Size:     int64(rev.Download.Size),
	}
	resourceURL, err := url.Parse(rev.Download.URL)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}
	return r, resourceURL, nil
}
