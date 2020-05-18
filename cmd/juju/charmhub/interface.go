// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

// InfoCommandAPI describes API methods required
// to execute the info command.
type InfoCommandAPI interface {
	Close() error
}
