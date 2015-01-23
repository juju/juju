// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/storage"
	"github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type storageSuite struct {
	// TODO(anastasiamac) mock to remove JujuConnSuite
	jujutesting.JujuConnSuite

	api        *storage.API
	authorizer testing.FakeAuthorizer
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.authorizer = testing.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	var err error
	s.api, err = storage.NewAPI(s.State, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestShowStorage(c *gc.C) {
	storageTag := "test-storage"
	entity := params.Entity{Tag: storageTag}

	found, err := s.api.Show(params.Entities{Entities: []params.Entity{entity}})
	c.Assert(err.Error(), gc.Matches, ".*not implemented.*")
	c.Assert(found.Results, gc.HasLen, 0)
	c.Assert(found.Results, gc.IsNil)
}
