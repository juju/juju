// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/url"

	"github.com/juju/errors"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

type charmHubOpener struct {
	modelConfigService ModelConfigService
}

type resourceClientGetter func(ctx context.Context, logger corelogger.Logger) (ResourceGetter, error)

func (rcg resourceClientGetter) GetResourceClient(ctx context.Context, logger corelogger.Logger) (ResourceGetter, error) {
	return rcg(ctx, logger)
}

func NewCharmHubOpener(modelConfigService ModelConfigService) resourceClientGetter {
	ch := &charmHubOpener{modelConfigService: modelConfigService}
	return ch.NewClient
}

func (ch *charmHubOpener) NewClient(ctx context.Context, logger corelogger.Logger) (ResourceGetter, error) {
	config, err := ch.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmhubURL, _ := config.CharmHubURL()
	client, err := newCharmHubClient(charmhubURL, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRetryClient(client, logger), nil
}

func newCharmHubClient(charmhubURL string, logger corelogger.Logger) (*CharmHubClient, error) {
	chClient, err := charmhub.NewClient(charmhub.Config{
		URL:    charmhubURL,
		Logger: logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CharmHubClient{client: chClient, logger: logger.Child("charmhub", corelogger.CHARMHUB)}, nil
}

type CharmHubClient struct {
	client CharmHub
	logger corelogger.Logger
}

// GetResource returns data about the resource including an io.ReadCloser
// to download the resource.  The caller is responsible for closing it.
func (ch *CharmHubClient) GetResource(req ResourceRequest) (ResourceData, error) {
	ch.logger.Tracef("GetResource(%s)", pretty.Sprint(req))
	var data ResourceData

	// GetResource is called after a charm is installed, therefore the
	// origin must have an ID. Error if not.
	origin := req.CharmID.Origin
	if origin.Revision == nil {
		return data, errors.BadRequestf("empty charm origin revision")
	}

	// The charm revision isn't really required here, just handy for
	// getting the correct resource revision. Using a channel would
	// limit resource revisions found. The resource revision is set
	// during deploy when a resolving resources for add pending resources.
	// This also closes a timing window where a charm and resource
	// is updated in the channel in between deploy and resource use.
	cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *origin.Revision)
	if err != nil {
		return data, errors.Trace(err)
	}
	if newCfg, ok := charmhub.AddResource(cfg, req.Name, req.Revision); ok {
		cfg = newCfg
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
