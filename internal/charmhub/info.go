// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/http"
	"runtime/pprof"
	"strings"

	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/charmhub/path"
	"github.com/juju/juju/internal/charmhub/transport"
)

// InfoOption to be passed to Info to customize the resulting request.
type InfoOption func(*infoOptions)

type infoOptions struct {
	channel *string
}

// WithInfoChannel sets the channel on the option.
func WithInfoChannel(ch string) InfoOption {
	return func(infoOptions *infoOptions) {
		infoOptions.channel = &ch
	}
}

// Create a infoOptions instance with default values.
func newInfoOptions() *infoOptions {
	return &infoOptions{}
}

// infoClient defines a client for info requests.
type infoClient struct {
	path   path.Path
	client RESTClient
	logger corelogger.Logger
}

// newInfoClient creates a infoClient for requesting
func newInfoClient(path path.Path, client RESTClient, logger corelogger.Logger) *infoClient {
	return &infoClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

// Info requests the information of a given charm. If that charm doesn't exist
// an error stating that fact will be returned.
func (c *infoClient) Info(ctx context.Context, name string, options ...InfoOption) (resp transport.InfoResponse, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("charmhub.name", name),
		trace.StringAttr("charmhub.request", "info"),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, span.Scope().TraceID()), func(ctx context.Context) {
		resp, err = c.info(ctx, name, options...)
	})
	return
}

func (c *infoClient) info(ctx context.Context, name string, options ...InfoOption) (transport.InfoResponse, error) {
	opts := newInfoOptions()
	for _, option := range options {
		option(opts)
	}

	isTraceEnabled := c.logger.IsLevelEnabled(corelogger.TRACE)
	if isTraceEnabled {
		c.logger.Tracef(ctx, "Info(%s)", name)
	}

	var resp transport.InfoResponse
	path, err := c.path.Join(name)
	if err != nil {
		return resp, errors.Trace(err)
	}

	path, err = path.Query("fields", defaultInfoFilter())
	if err != nil {
		return resp, errors.Trace(err)
	}

	if opts.channel != nil {
		path, err = path.Query("channel", *opts.channel)
		if err != nil {
			return resp, errors.Trace(err)
		}
	}

	restResp, err := c.client.Get(ctx, path, &resp)
	if err != nil {
		return resp, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return resp, errors.NotFoundf(name)
	}
	if err := handleBasicAPIErrors(ctx, resp.ErrorList, c.logger); err != nil {
		return resp, errors.Trace(err)
	}

	switch resp.Type {
	case transport.CharmType, transport.BundleType:
	default:
		return resp, errors.Errorf("unexpected response type %q, expected charm or bundle", resp.Type)
	}

	if isTraceEnabled {
		c.logger.Tracef(ctx, "Info() unmarshalled: %+v", resp)
	}
	return resp, nil
}

// defaultInfoFilter returns a filter string to retrieve all data
// necessary to fill the transport.InfoResponse.  Without it, we'd
// receive the Name, ID and Type.
func defaultInfoFilter() string {
	filter := defaultResultFilter
	filter = append(filter, "default-release.revision.download.size")
	filter = append(filter, appendFilterList("default-release", infoDefaultRevisionFilter)...)
	filter = append(filter, appendFilterList("default-release", defaultChannelFilter)...)
	filter = append(filter, "channel-map.revision.download.size")
	filter = append(filter, appendFilterList("channel-map", infoChannelMapRevisionFilter)...)
	filter = append(filter, appendFilterList("channel-map", defaultChannelFilter)...)
	return strings.Join(filter, ",")
}

var infoDefaultRevisionFilter = []string{
	"revision.config-yaml",
	"revision.metadata-yaml",
	"revision.bundle-yaml",
	"revision.bases.architecture",
	"revision.bases.name",
	"revision.bases.channel",
	"revision.revision",
	"revision.version",
}

var infoChannelMapRevisionFilter = []string{
	"revision.created-at",
	"revision.bases.architecture",
	"revision.bases.name",
	"revision.bases.channel",
	"revision.revision",
	"revision.version",
}
