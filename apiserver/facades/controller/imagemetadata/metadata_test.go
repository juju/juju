// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/imagemetadata"
	apiservertesting "github.com/juju/juju/apiserver/testing"
)

type ImageMetadataUpdateSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ImageMetadataUpdateSuite{})

func (s *ImageMetadataUpdateSuite) TestControllerOnly(c *tc.C) {
	var authorizer apiservertesting.FakeAuthorizer
	authorizer.Controller = true
	_, err := imagemetadata.NewAPI(facadetest.ModelContext{
		Auth_: authorizer,
	})
	c.Assert(err, tc.ErrorIsNil)
	authorizer.Controller = false
	_, err = imagemetadata.NewAPI(facadetest.ModelContext{
		Auth_: authorizer,
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
