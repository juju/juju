// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stream

import (
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
)

// Config exposes the information necessary to open a streaming
// connection to an API endpoint.
type Config interface {
	// Endpoint is the API endpoint path to which to connect.
	Endpoint() string

	// Apply adjusts the provided URL query to match the config.
	Apply(url.Values)
}

// Open opens a streaming connection that conforms to the provided
// config (and its endpoint).
func Open(conn base.StreamConnector, cfg Config) (base.Stream, error) {
	path := cfg.Endpoint()
	attrs := make(url.Values)
	cfg.Apply(attrs)
	stream, err := conn.ConnectStream(path, attrs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot connect to %s", path)
	}
	return stream, nil
}
