// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/url"

	jujuerrors "github.com/juju/errors"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/resource/downloader"
)

type charmHubOpener struct {
	modelConfigService ModelConfigService
}

type resourceClientGetter func(ctx context.Context, logger corelogger.Logger) (ResourceClient, error)

func (rcg resourceClientGetter) GetResourceClient(ctx context.Context, logger corelogger.Logger) (ResourceClient, error) {
	return rcg(ctx, logger)
}

func NewCharmHubOpener(modelConfigService ModelConfigService) resourceClientGetter {
	ch := &charmHubOpener{
		modelConfigService: modelConfigService,
	}
	return ch.NewClient
}

func (ch *charmHubOpener) NewClient(ctx context.Context, logger corelogger.Logger) (ResourceClient, error) {
	config, err := ch.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	charmhubURL, _ := config.CharmHubURL()
	client, err := newCharmHubClient(charmhubURL, logger)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return NewRetryClient(client, logger), nil
}

func newCharmHubClient(charmhubURL string, logger corelogger.Logger) (*CharmHubClient, error) {
	chClient, err := charmhub.NewClient(charmhub.Config{
		URL:    charmhubURL,
		Logger: logger,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &CharmHubClient{
		client:     chClient,
		downloader: downloader.NewResourceDownloader(chClient, logger),
		logger:     logger.Child("charmhub", corelogger.CHARMHUB),
	}, nil
}

type CharmHubClient struct {
	client     CharmHub
	downloader Downloader
	logger     corelogger.Logger
}

// GetResource returns data about the resource including an io.ReadCloser
// to download the resource.  The caller is responsible for closing it.
func (ch *CharmHubClient) GetResource(ctx context.Context, req ResourceRequest) (ResourceData, error) {
	ch.logger.Tracef(ctx, "GetResource(%s)", pretty.Sprint(req))

	res, resourceURL, err := ch.getResourceDetails(ctx, req)
	if err != nil {
		return ResourceData{}, errors.Capture(err)
	}

	ch.logger.Tracef(ctx, "Read resource %q from %q", res.Name, resourceURL)

	readCloser, err := ch.downloader.Download(ctx, resourceURL, res.Fingerprint.String(), res.Size)
	if err != nil {
		return ResourceData{}, errors.Errorf("downloading resource: %w", err)
	}
	return ResourceData{
		Resource:   res,
		ReadCloser: readCloser,
	}, nil
}

// getResourceDetails fetches information about the specified resource from
// charmhub.
func (ch *CharmHubClient) getResourceDetails(ctx context.Context, req ResourceRequest) (charmresource.Resource, *url.URL, error) {
	// GetResource is called after a charm is installed, therefore the
	// origin must have an ID. Error if not.
	origin := req.CharmID.Origin
	if origin.Revision == nil {
		return charmresource.Resource{}, nil, jujuerrors.BadRequestf("empty charm origin revision")
	}

	// The charm revision isn't really required here, just handy for
	// getting the correct resource revision. Using a channel would
	// limit resource revisions found. The resource revision is set
	// during deploy when a resolving resources for add pending resources.
	// This also closes a timing window where a charm and resource
	// is updated in the channel in between deploy and resource use.
	cfg, err := charmhub.DownloadOneFromRevision(ctx, origin.ID, *origin.Revision)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Capture(err)
	}
	if newCfg, ok := charmhub.AddResource(cfg, req.Name, req.Revision); ok {
		cfg = newCfg
	}
	refreshResp, err := ch.client.Refresh(ctx, cfg)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Capture(err)
	}
	if len(refreshResp) == 0 {
		return charmresource.Resource{}, nil, errors.Errorf("no download refresh responses received")
	}
	resp := refreshResp[0]
	return resourceFromRevision(req.Name, resp.Entity.Resources)
}

// resourceFromRevision finds the information about the specified resource
// revision in the transport resource revision response.
func resourceFromRevision(name string, revs []transport.ResourceRevision) (charmresource.Resource, *url.URL, error) {
	var rev transport.ResourceRevision
	for _, v := range revs {
		if v.Name == name {
			rev = v
		}
	}
	if rev.Name != name {
		return charmresource.Resource{}, nil, errors.Capture(jujuerrors.NotFoundf("resource %q", name))
	}
	resType, err := charmresource.ParseType(rev.Type)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Capture(err)
	}
	fingerprint, err := charmresource.ParseFingerprint(rev.Download.HashSHA384)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Capture(err)
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
		return charmresource.Resource{}, nil, errors.Capture(err)
	}
	return r, resourceURL, nil
}
