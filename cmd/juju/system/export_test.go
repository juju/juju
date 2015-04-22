// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"github.com/juju/juju/environs/configstore"
)

// NewListCommand returns a ListCommand with the configstore provided as specified.
func NewListCommand(cfgStore configstore.Storage) *ListCommand {
	return &ListCommand{
		cfgStore: cfgStore,
	}
}
