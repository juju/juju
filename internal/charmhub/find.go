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

// FindOption to be passed to Find to customize the resulting request.
type FindOption func(*findOptions)

type findOptions struct {
	category         *string
	channel          *string
	charmType        *string
	platforms        *string
	publisher        *string
	relationRequires *string
	relationProvides *string
}

// WithFindCategory sets the category on the option.
func WithFindCategory(category string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.category = &category
	}
}

// WithFindChannel sets the channel on the option.
func WithFindChannel(channel string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.channel = &channel
	}
}

// WithFindType sets the charmType on the option.
func WithFindType(charmType string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.charmType = &charmType
	}
}

// WithFindPlatforms sets the charmPlatforms on the option.
func WithFindPlatforms(platforms string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.platforms = &platforms
	}
}

// WithFindPublisher sets the publisher on the option.
func WithFindPublisher(publisher string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.publisher = &publisher
	}
}

// WithFindRelationRequires sets the relationRequires on the option.
func WithFindRelationRequires(relationRequires string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.relationRequires = &relationRequires
	}
}

// WithFindRelationProvides sets the relationProvides on the option.
func WithFindRelationProvides(relationProvides string) FindOption {
	return func(findOptions *findOptions) {
		findOptions.relationProvides = &relationProvides
	}
}

// Create a findOptions instance with default values.
func newFindOptions() *findOptions {
	return &findOptions{}
}

// findClient defines a client for querying information about a given charm or
// bundle for a given CharmHub store.
type findClient struct {
	path   path.Path
	client RESTClient
	logger corelogger.Logger
}

// newFindClient creates a findClient for querying charm or bundle information.
func newFindClient(path path.Path, client RESTClient, logger corelogger.Logger) *findClient {
	return &findClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

// Find searches Charm Hub and provides results matching a string.
func (c *findClient) Find(ctx context.Context, query string, options ...FindOption) (result []transport.FindResponse, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("charmhub.query", query),
		trace.StringAttr("charmhub.request", "find"),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, span.Scope().TraceID()), func(ctx context.Context) {
		result, err = c.find(ctx, query, options...)
	})
	return
}

func (c *findClient) find(ctx context.Context, query string, options ...FindOption) ([]transport.FindResponse, error) {
	opts := newFindOptions()
	for _, option := range options {
		option(opts)
	}

	c.logger.Tracef(ctx, "Find(%s)", query)
	path, err := c.path.Query("q", query)
	if err != nil {
		return nil, errors.Trace(err)
	}

	path, err = path.Query("fields", defaultFindFilter())
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := walkFindOptions(opts, func(name, value string) error {
		path, err = path.Query(name, value)
		return errors.Trace(err)
	}); err != nil {
		return nil, errors.Trace(err)
	}

	var resp transport.FindResponses
	restResp, err := c.client.Get(ctx, path, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf(query)
	}
	if err := handleBasicAPIErrors(ctx, resp.ErrorList, c.logger); err != nil {
		return nil, errors.Trace(err)
	}

	return resp.Results, nil
}

func walkFindOptions(opts *findOptions, fn func(string, string) error) error {
	// We could use reflect here, but it might be easier to just list out what
	// we want to walk over.
	// See: https://gist.github.com/SimonRichardson/7c9243d71551cad4af7661128add93b5
	if opts.category != nil {
		if err := fn("category", *opts.category); err != nil {
			return errors.Trace(err)
		}
	}
	if opts.channel != nil {
		if err := fn("channel", *opts.channel); err != nil {
			return errors.Trace(err)
		}
	}
	if opts.charmType != nil {
		if err := fn("type", *opts.charmType); err != nil {
			return errors.Trace(err)
		}
	}
	if opts.platforms != nil {
		if err := fn("platforms", *opts.platforms); err != nil {
			return errors.Trace(err)
		}
	}
	if opts.publisher != nil {
		if err := fn("publisher", *opts.publisher); err != nil {
			return errors.Trace(err)
		}
	}
	if opts.relationRequires != nil {
		if err := fn("relation-requires", *opts.relationRequires); err != nil {
			return errors.Trace(err)
		}
	}
	if opts.relationProvides != nil {
		if err := fn("relation-provides", *opts.relationProvides); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// defaultFindFilter returns a filter string to retrieve all data
// necessary to fill the transport.FindResponse.  Without it, we'd
// receive the Name, ID and Type.
func defaultFindFilter() string {
	filter := defaultFindResultFilter
	filter = append(filter, appendFilterList("default-release", defaultRevisionFilter)...)
	return strings.Join(filter, ",")
}

var defaultFindResultFilter = []string{
	"result.publisher.display-name",
	"result.summary",
	"result.store-url",
}

var defaultRevisionFilter = []string{
	"revision.bases.architecture",
	"revision.bases.name",
	"revision.bases.channel",
	"revision.version",
}
