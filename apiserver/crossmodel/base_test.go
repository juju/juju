// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type baseCrossmodelSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer testing.FakeAuthorizer

	api *crossmodel.API

	calls []string
}

func (s *baseCrossmodelSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authorizer = testing.FakeAuthorizer{names.NewUserTag("testuser"), true}

	var err error
	s.api, err = crossmodel.NewAPI(nil, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}
