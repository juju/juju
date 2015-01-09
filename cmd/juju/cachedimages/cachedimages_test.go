// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachedimages_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cachedimages"
	"github.com/juju/juju/testing"
)

type cachedImagesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&cachedImagesSuite{})

var expectedCachedImagesCommmandNames = []string{
	"delete",
	"help",
	"list",
}

func (s *cachedImagesSuite) TestHelp(c *gc.C) {
	// Check the help output
	ctx, err := testing.RunCommand(c, cachedimages.NewSuperCommand(), "--help")
	c.Assert(err, jc.ErrorIsNil)
	namesFound := testing.ExtractCommandsFromHelpOutput(ctx)
	c.Assert(namesFound, gc.DeepEquals, expectedCachedImagesCommmandNames)
}
