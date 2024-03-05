// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"context"

	"github.com/google/go-querystring/query"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
)

// Open opens a streaming connection to the endpoint path that conforms
// to the provided config.
func Open(ctx context.Context, conn base.StreamConnector, path string, cfg interface{}) (base.Stream, error) {
	attrs, err := query.Values(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate URL query from config")
	}
	stream, err := conn.ConnectStream(ctx, path, attrs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to %s", path)
	}
	return stream, nil
}
