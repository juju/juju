// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/featureflag"
	"github.com/juju/juju/jujuclient"
	jujutesting "github.com/juju/juju/testing"
)

// None of the tests in this package require mongo.
// Full command integration tests are found in cmd/juju/model_test.go

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type generationBaseSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	branchName string
}

func (s *generationBaseSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.SetFeatureFlags(featureflag.Branches)
	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}
	err := s.store.UpdateModel("testing", "admin/mymodel", jujuclient.ModelDetails{
		ModelUUID:    jujutesting.ModelTag.Id(),
		ModelType:    model.IAAS,
		ActiveBranch: model.GenerationMaster,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].CurrentModel = "admin/mymodel"

	s.branchName = "new-branch"
}
