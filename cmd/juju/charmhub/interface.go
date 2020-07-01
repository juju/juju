// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import "github.com/juju/juju/api/charmhub"

// InfoCommandAPI describes API methods required
// to execute the info command.
type InfoCommandAPI interface {
	Info(string) (charmhub.InfoResponse, error)
	Close() error
}
