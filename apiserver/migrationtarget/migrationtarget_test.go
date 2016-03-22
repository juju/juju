// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationtarget_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/migrationtarget"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/description"
	_ "github.com/juju/juju/provider/dummy"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
)

type Suite struct {
	statetesting.StateSuite
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&Suite{})

func (s *Suite) SetUpTest(c *gc.C) {
	// Required to allow model import test to work.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"type":     "dummy",
		"state-id": "42",
	})
	s.StateSuite.SetUpTest(c)

	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.Owner,
	}
}

func (s *Suite) TestFacadeRegistered(c *gc.C) {
	factory, err := common.Facades.GetFactory("MigrationTarget", 1)
	c.Assert(err, jc.ErrorIsNil)

	api, err := factory(s.State, s.resources, s.authorizer, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(api, gc.FitsTypeOf, new(migrationtarget.API))
}

func (s *Suite) TestNotUser(c *gc.C) {
	s.authorizer.Tag = names.NewMachineTag("0")
	_, err := s.newAPI()
	c.Assert(errors.Cause(err), gc.Equals, common.ErrPerm)
}

func (s *Suite) TestNotControllerAdmin(c *gc.C) {
	s.authorizer.Tag = names.NewUserTag("jrandomuser")
	_, err := s.newAPI()
	c.Assert(errors.Cause(err), gc.Equals, common.ErrPerm)
}

func (s *Suite) TestImport(c *gc.C) {
	uuid, bytes := s.makeExportedModel(c)
	api := s.mustNewAPI(c)

	err := api.Import(params.SerializedModel{Bytes: bytes})
	c.Assert(err, jc.ErrorIsNil)

	// Check the model was imported.
	st, err := s.State.ForModel(names.NewModelTag(uuid))
	c.Assert(err, jc.ErrorIsNil)
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Name(), gc.Equals, "some-model")
}

func (s *Suite) newAPI() (*migrationtarget.API, error) {
	return migrationtarget.NewAPI(s.State, s.resources, s.authorizer)
}

func (s *Suite) mustNewAPI(c *gc.C) *migrationtarget.API {
	api, err := s.newAPI()
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *Suite) makeExportedModel(c *gc.C) (string, []byte) {
	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	newUUID := utils.MustNewUUID().String()
	model.UpdateConfig(map[string]interface{}{
		"name":    "some-model",
		"uuid":    newUUID,
		"ca-cert": "not really a cert",
	})

	bytes, err := description.Serialize(model)
	c.Assert(err, jc.ErrorIsNil)
	return newUUID, bytes
}
