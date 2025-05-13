// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"

	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

func TestAll(t *testing.T) {
	tc.TestingT(t)
}

type BaseStorageSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
}

func (s *BaseStorageSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
}

func (s *BaseStorageSuite) TearDownTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

type SubStorageSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore
}

func (s *SubStorageSuite) SetUpTest(c *tc.C) {
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
