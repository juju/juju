// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"github.com/google/go-querystring/query"
	"github.com/juju/errors"

	"github.com/juju/juju/v3/api/base"
)

// Open opens a streaming connection to the endpoint path that conforms
// to the provided config.
func Open(conn base.StreamConnector, path string, cfg interface{}) (base.Stream, error) {
	attrs, err := query.Values(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "failed to generate URL query from config")
	}
	stream, err := conn.ConnectStream(path, attrs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to %s", path)
	}
	return stream, nil
}
