// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/url"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/charmhub/transport"
)

// Printer defines an interface for printing out values.
type Printer interface {
	Print() error
}

// Log describes a log format function to output to.
type Log = func(format string, params ...interface{})

// InfoCommandAPI describes API methods required to execute the info command.
type InfoCommandAPI interface {
	Info(string) (charmhub.InfoResponse, error)
	Close() error
}

// FindCommandAPI describes API methods required to execute the find command.
type FindCommandAPI interface {
	Find(string) ([]charmhub.FindResponse, error)
	Close() error
}

// DownloadCommandAPI describes API methods required to execute the download
// command.
type DownloadCommandAPI interface {
	Info(context.Context, string) (transport.InfoResponse, error)
	Download(context.Context, *url.URL, string) error
}

type ModelConfigGetter interface {
	ModelGet() (map[string]interface{}, error)
}
