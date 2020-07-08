// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import "github.com/juju/juju/api/charmhub"

// Printer defines what is needed to print info.
type Printer interface {
	Print() error
}

// Log describes a log format function to output to.
type Log = func(format string, params ...interface{})

// InfoCommandAPI describes API methods required
// to execute the info command.
type InfoCommandAPI interface {
	Info(string) (charmhub.InfoResponse, error)
	Close() error
}

// FindCommandAPI describes API methods required
// to execute the info command.
type FindCommandAPI interface {
	Find(string) ([]charmhub.FindResponse, error)
	Close() error
}
