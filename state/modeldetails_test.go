// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"sort"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
)

type ModelDetailsSuite struct {
	ConnSuite
}

var _ = gc.Suite(&ModelDetailsSuite{})

func (s *ModelDetailsSuite) Setup3Models(c *gc.C) {
	user1 := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "user1",
		NoModelUser: true,
	})
	st1 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "user1model",
		Owner: user1.Tag(),
	})
	st1.Close()
	user2 := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "user2",
		NoModelUser: true,
	})
	st2 := s.Factory.MakeModel(c, &factory.ModelParams{
		Name:  "user2model",
		Owner: user2.Tag(),
	})
	st2.Close()
	owner := s.Model.Owner()
	sharedSt := s.Factory.MakeModel(c, &factory.ModelParams{
		Name: "shared",
		// Owned by test-admin
		Owner: owner,
	})
	defer sharedSt.Close()
	sharedModel, err := sharedSt.Model()
	c.Assert(err, jc.ErrorIsNil)
	_, err = sharedModel.AddUser(state.UserAccessSpec{
		User:      user1.UserTag(),
		CreatedBy: owner,
		Access:    "write",
	})
	c.Assert(err, jc.ErrorIsNil)
	// User 2 has read access to the shared model
	_, err = sharedModel.AddUser(state.UserAccessSpec{
		User:      user2.UserTag(),
		CreatedBy: owner,
		Access:    "read",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelDetailsSuite) modelNamesForUser(c *gc.C, user string) []string {
	tag := names.NewUserTag(user)
	modelQuery, closer, err := s.State.ModelQueryForUser(tag)
	defer closer()
	c.Assert(err, jc.ErrorIsNil)
	var docs []struct {
		Name string `bson:"name"`
	}
	modelQuery.Select(bson.M{"name": 1})
	err = modelQuery.All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	names := make([]string, 0)
	for _, doc := range docs {
		names = append(names, doc.Name)
	}
	sort.Strings(names)
	return names
}

func (s *ModelDetailsSuite) TestModelsForUserAdmin(c *gc.C) {
	s.Setup3Models(c)
	names := s.modelNamesForUser(c, s.Model.Owner().Name())
	// Admin always gets to see all models
	c.Check(names, gc.DeepEquals, []string{"shared", "testenv", "user1model", "user2model"})
}

func (s *ModelDetailsSuite) TestModelsForUser1(c *gc.C) {
	// User1 is only added to the model they own and the shared model
	s.Setup3Models(c)
	names := s.modelNamesForUser(c, "user1")
	// Admin always gets to see all models
	c.Check(names, gc.DeepEquals, []string{"shared", "user1model"})
}
