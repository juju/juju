// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type externalControllerSuite struct {
	ConnSuite
	externalControllers state.ExternalControllers
}

var _ = gc.Suite(&externalControllerSuite{})

func (s *externalControllerSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.externalControllers = state.NewExternalControllers(s.State)
}

func (s *externalControllerSuite) TestSaveInvalidAddress(c *gc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0"},
		CACert:        testing.CACert,
	}
	_, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`controller api address "192.168.1.0" not valid`))
}

func (s *externalControllerSuite) TestSaveNoModels(c *gc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0:1234", "10.0.0.1:1234"},
		CACert:        testing.CACert,
	}
	ec, err := s.externalControllers.Save(controllerInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec.Id(), gc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), jc.DeepEquals, controllerInfo)
	s.assertSavedControllerInfo(c)
}

func (s *externalControllerSuite) assertSavedControllerInfo(c *gc.C, modelUUIDs ...string) {
	coll, closer := state.GetCollection(s.State, "externalControllers")
	defer closer()

	var raw bson.M
	err := coll.FindId(testing.ControllerTag.Id()).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(raw["_id"], gc.Equals, testing.ControllerTag.Id())
	c.Assert(raw["addresses"], jc.SameContents, []interface{}{"192.168.1.0:1234", "10.0.0.1:1234"})
	c.Assert(raw["cacert"], gc.Equals, testing.CACert)
	var models []string
	for _, m := range raw["models"].([]interface{}) {
		models = append(models, m.(string))
	}
	c.Assert(models, jc.SameContents, modelUUIDs)
}

func (s *externalControllerSuite) TestSave(c *gc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0:1234", "10.0.0.1:1234"},
		CACert:        testing.CACert,
	}
	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	ec, err := s.externalControllers.Save(controllerInfo, uuid1, uuid2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec.Id(), gc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), jc.DeepEquals, controllerInfo)
	s.assertSavedControllerInfo(c, uuid1, uuid2)
}

func (s *externalControllerSuite) TestSaveIdempotent(c *gc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0:1234", "10.0.0.1:1234"},
		CACert:        testing.CACert,
	}
	uuid1 := utils.MustNewUUID().String()
	ec, err := s.externalControllers.Save(controllerInfo, uuid1)
	c.Assert(err, jc.ErrorIsNil)
	ec, err = s.externalControllers.Save(controllerInfo, uuid1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec.Id(), gc.Equals, testing.ControllerTag.Id())
	c.Assert(ec.ControllerInfo(), jc.DeepEquals, controllerInfo)
	s.assertSavedControllerInfo(c, uuid1)
}

func (s *externalControllerSuite) TestUpdateModels(c *gc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0:1234", "10.0.0.1:1234"},
		CACert:        testing.CACert,
	}
	uuid1 := utils.MustNewUUID().String()
	_, err := s.externalControllers.Save(controllerInfo, uuid1)
	c.Assert(err, jc.ErrorIsNil)
	uuid2 := utils.MustNewUUID().String()
	_, err = s.externalControllers.Save(controllerInfo, uuid2)
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedControllerInfo(c, uuid1, uuid2)
}

func (s *externalControllerSuite) TestControllerForModel(c *gc.C) {
	controllerInfo := crossmodel.ControllerInfo{
		ControllerTag: testing.ControllerTag,
		Addrs:         []string{"192.168.1.0:1234", "10.0.0.1:1234"},
		CACert:        testing.CACert,
	}
	uuid1 := utils.MustNewUUID().String()
	uuid2 := utils.MustNewUUID().String()
	ec, err := s.externalControllers.Save(controllerInfo, uuid1, uuid2)
	c.Assert(err, jc.ErrorIsNil)
	found, err := s.externalControllers.ControllerForModel(uuid1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ec, jc.DeepEquals, found)
	_, err = s.externalControllers.ControllerForModel("1234")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
