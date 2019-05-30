// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/settings"
)

type BranchSuite struct {
	cache.EntitySuite
}

var _ = gc.Suite(&BranchSuite{})

var branchChange = cache.BranchChange{
	ModelUUID:     "model-uuid",
	Name:          "testing-branch",
	AssignedUnits: map[string][]string{"redis": {"redis/0", "redis/1"}},
	Config:        map[string][]settings.ItemChange{"redis": {settings.MakeAddition("password", "pass666")}},
	Completed:     0,
	GenerationId:  0,
}
