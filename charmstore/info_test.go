// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore_test

import (
	"github.com/juju/charm/v7"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
)

type CharmInfoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmInfoSuite{})

func (CharmInfoSuite) TestLatestURL(c *gc.C) {
	info := charmstore.CharmInfo{
		OriginalURL:    charm.MustParseURL("cs:quantal/mysql-3"),
		LatestRevision: 17,
	}

	latestURL := info.LatestURL()

	c.Check(latestURL, jc.DeepEquals, charm.MustParseURL("cs:quantal/mysql-17"))
}
