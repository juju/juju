// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	"github.com/juju/juju/core/cache"
)

// Mostly a placeholder file at this stage.

var relationChange = cache.RelationChange{
	ModelUUID: "model-uuid",
	Key:       "provider:ep consumer:ep",
	Endpoints: []cache.Endpoint{
		{
			Application: "provider",
			Name:        "ep",
			Role:        "provider",
			Interface:   "foo",
		}, {
			Application: "consumer",
			Name:        "ep",
			Role:        "requires",
			Interface:   "foo",
		},
	},
}
