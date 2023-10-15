// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/jujuclient"
	jujutesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type BaseStorageSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
}

func (s *BaseStorageSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *BaseStorageSuite) TearDownTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

type SubStorageSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func (s *SubStorageSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {},
		},
		CurrentModel: "admin/controller",
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
}
