// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/testing"
)

type initSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&initSuite{})

func (s *initSuite) TestImageMetadataDatasourceAdded(c *gc.C) {
	env := azure.MakeEnvironForTest(c)
	dss, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)

	expected := "cloud local storage"
	found := false
	for i, ds := range dss {
		c.Logf("datasource %d: %+v", i, ds)
		if ds.Description() == expected {
			found = true
			break
		}
	}
	c.Assert(found, jc.IsTrue)
}
