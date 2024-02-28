// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/imagemetadata"
	apiservertesting "github.com/juju/juju/apiserver/testing"
)

type ImageMetadataUpdateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ImageMetadataUpdateSuite{})

func (s *ImageMetadataUpdateSuite) TestControllerOnly(c *gc.C) {
	var authorizer apiservertesting.FakeAuthorizer
	authorizer.Controller = true
	_, err := imagemetadata.NewAPI(facadetest.ModelContext{
		Auth_: authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	authorizer.Controller = false
	_, err = imagemetadata.NewAPI(facadetest.ModelContext{
		Auth_: authorizer,
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
