// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"net/url"
	"os"

	jujuerrors "github.com/juju/errors"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/errors"
)

const (
	// ErrUnexpectedFingerprint is returned when the fingerprint of the downloaded
	// resource does not match the expected fingerprint.
	ErrUnexpectedFingerprint = errors.ConstError("downloaded resource has unexpected fingerprint")

	// ErrUnexpectedSize is returned when the size of the downloaded
	// resources does not match the expected size.
	ErrUnexpectedSize = errors.ConstError("downloaded resource has unexpected size")
)

type charmHubOpener struct {
	modelConfigService ModelConfigService
}

type resourceClientGetter func(ctx context.Context, logger corelogger.Logger) (ResourceClient, error)

func (rcg resourceClientGetter) GetResourceClient(ctx context.Context, logger corelogger.Logger) (ResourceClient, error) {
	return rcg(ctx, logger)
}

func NewCharmHubOpener(modelConfigService ModelConfigService) resourceClientGetter {
	ch := &charmHubOpener{modelConfigService: modelConfigService}
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
	return &CharmHubClient{client: chClient, logger: logger.Child("charmhub", corelogger.CHARMHUB)}, nil
}

type CharmHubClient struct {
	client CharmHub
	logger corelogger.Logger
}

// GetResource returns data about the resource including an io.ReadCloser
// to download the resource.  The caller is responsible for closing it.
func (ch *CharmHubClient) GetResource(ctx context.Context, req ResourceRequest) (ResourceData, error) {
	ch.logger.Tracef("GetResource(%s)", pretty.Sprint(req))

	res, resourceURL, err := ch.getResourceDetails(ctx, req)
	if err != nil {
		return ResourceData{}, errors.Capture(err)
	}

	ch.logger.Tracef("Read resource %q from %q", res.Name, resourceURL)

	tmpFile, err := os.CreateTemp("", "resource-")
	if err != nil {
		return ResourceData{}, errors.Capture(err)
	}
	defer func() {
		// Always close the file, we no longer require to have it open for
		// this purpose. Another process/method can take over the file.
		tmpFile.Close()

		// If the download was successful, we don't need to remove the file.
		// It is the responsibility of the caller to remove the file.
		if err == nil {
			return
		}

		// Remove the temporary file if the download failed. If we can't
		// remove the file, log a warning, so the operator can clean it up
		// manually.
		removeErr := os.Remove(tmpFile.Name())
		if removeErr != nil {
			ch.logger.Warningf("failed to remove temporary file %q: %v", tmpFile.Name(), removeErr)
		}
	}()

	downloadResult, err := ch.download(ctx, resourceURL, tmpFile)
	if err != nil {
		return ResourceData{}, errors.Capture(err)
	}
	if downloadResult.SHA384 != res.Fingerprint.String() {
		return ResourceData{}, errors.Errorf(
			"%w: %q, got %q", ErrUnexpectedFingerprint, res.Fingerprint.String(), downloadResult.SHA384,
		)
	}
	if downloadResult.Size != res.Size {
		return ResourceData{}, errors.Errorf(
			"%w: %q, got %q", ErrUnexpectedSize, res.Size, downloadResult.Size,
		)
	}

	// Create a reader for the temporary file containing the resource.
	tmpFileReader, err := newTmpFileReader(tmpFile.Name(), ch.logger)
	if err != nil {
		return ResourceData{}, errors.Errorf("opening downloaded resource: %w", err)
	}

	return ResourceData{
		Resource:   res,
		ReadCloser: tmpFileReader,
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
	cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *origin.Revision)
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

// Download looks up the requested charm using the appropriate store, downloads
// it to a temporary file and passes it to the configured storage API so it can
// be persisted.
//
// The resulting charm is verified to be the right hash. It expected that the
// origin will always have the correct hash following this call.
//
// Returns [ErrInvalidHash] if the hash of the downloaded charm does not match
// the expected hash.
func (d *CharmHubClient) download(ctx context.Context, url *url.URL, tmpFile *os.File) (_ *DownloadResult, err error) {
	d.logger.Debugf("downloading resource: %s", url)

	// Force the sha256 digest to be calculated on download.
	digest, err := d.client.Download(ctx, url, tmpFile.Name())
	if err != nil {
		return nil, errors.Capture(err)
	}

	d.logger.Debugf("downloaded charm: %q", url)

	return &DownloadResult{
		SHA256: digest.SHA256,
		SHA384: digest.SHA384,
		Size:   digest.Size,
	}, nil
}

// DownloadResult contains information about a downloaded charm.
type DownloadResult struct {
	ReadCloser io.ReadCloser
	SHA256     string
	SHA384     string
	Size       int64
}

// tmpFileReader wraps an *os.File and deletes it when closed.
type tmpFileReader struct {
	logger corelogger.Logger
	*os.File
}

func newTmpFileReader(file string, logger corelogger.Logger) (*tmpFileReader, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	return &tmpFileReader{
		logger: logger,
		File:   f,
	}, nil
}

// Close closes the temporary file and removes it. If the file cannot be
// removed, an error is logged.
func (f *tmpFileReader) Close() (err error) {
	defer func() {
		removeErr := os.Remove(f.Name())
		if err == nil {
			err = removeErr
		} else if removeErr != nil {
			f.logger.Warningf("failed to remove temporary file %q: %v", f.Name(), removeErr)
		}
	}()

	return f.File.Close()
}
