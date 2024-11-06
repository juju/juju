// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/charmhub/path"
	"github.com/juju/juju/internal/charmhub/transport"
)

// resourcesClient defines a client for resources requests.
type resourcesClient struct {
	path   path.Path
	client RESTClient
	logger corelogger.Logger
}

// newResourcesClient creates a resourcesClient for requesting
func newResourcesClient(path path.Path, client RESTClient, logger corelogger.Logger) *resourcesClient {
	return &resourcesClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

// ListResourceRevisions returns a slice of resource revisions for the provided
// resource of the given charm.
func (c *resourcesClient) ListResourceRevisions(ctx context.Context, charm, resource string) (_ []transport.ResourceRevision, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("charmhub.charm", charm),
		trace.StringAttr("charmhub.resource", resource),
		trace.StringAttr("charmhub.request", "list-resource-revisions"),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	isTraceEnabled := c.logger.IsLevelEnabled(corelogger.TRACE)
	if isTraceEnabled {
		c.logger.Tracef(ctx, "ListResourceRevisions(%s, %s)", charm, resource)
	}

	var resp transport.ResourcesResponse
	path, err := c.path.Join(charm, resource, "revisions")
	if err != nil {
		return nil, errors.Trace(err)
	}
	restResp, err := c.client.Get(ctx, path, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf("%q for %q", charm, resource)
	}

	if isTraceEnabled {
		c.logger.Tracef(ctx, "ListResourceRevisions(%s, %s) unmarshalled: %s", charm, resource, pretty.Sprint(resp.Revisions))
	}
	return resp.Revisions, nil
}
