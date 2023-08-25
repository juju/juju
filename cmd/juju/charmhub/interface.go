// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/url"

	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

// Printer defines an interface for printing out values.
type Printer interface {
	Print() error
}

// Log describes a log format function to output to.
type Log = func(format string, params ...interface{})

// CharmHubClient represents a CharmHub Client for making queries to the CharmHub API.
type CharmHubClient interface {
	URL() string
	Info(ctx context.Context, name string, options ...charmhub.InfoOption) (transport.InfoResponse, error)
	Find(ctx context.Context, query string, options ...charmhub.FindOption) ([]transport.FindResponse, error)
	Refresh(context.Context, charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
	Download(ctx context.Context, resourceURL *url.URL, archivePath string, options ...charmhub.DownloadOption) error
}
