// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lxdprofile"
)

type CharmSuite struct {
	cache.EntitySuite
}

var _ = gc.Suite(&CharmSuite{})

var charmChange = cache.CharmChange{
	ModelUUID: "model-uuid",
	CharmURL:  "www.charm-url.com-1",
	LXDProfile: lxdprofile.Profile{
		Config: map[string]string{"key": "value"},
	},
	DefaultConfig: map[string]interface{}{
		"key":       "default-value",
		"something": "else",
	},
}
