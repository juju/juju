// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/juju/environs"
)

// Filtering exports
var (
	MatchPortRanges = matchPortRanges
	MatchSubnet     = matchSubnet
)

func SetNewEnviron(c *Client, newEnviron func(context.Context) (environs.BootstrapEnviron, error)) {
	c.newEnviron = newEnviron
}
