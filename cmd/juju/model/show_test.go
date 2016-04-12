// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for info.

package model_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type ShowCommandSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake  fakeModelShowClient
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&ShowCommandSuite{})

type fakeModelShowClient struct {
	gitjujutesting.Stub
	info params.ModelInfo
	err  *params.Error
}

func (f *fakeModelShowClient) Close() error {
	f.MethodCall(f, "Close")
	return f.NextErr()
}

func (f *fakeModelShowClient) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	f.MethodCall(f, "ModelInfo", tags)
	if len(tags) != 1 {
		return nil, errors.Errorf("expected 1 tag, got %d", len(tags))
	}
	if tags[0] != testing.ModelTag {
		return nil, errors.Errorf("expected %s, got %s", testing.ModelTag.Id(), tags[0].Id())
	}
	return []params.ModelInfoResult{{Result: &f.info, Error: f.err}}, f.NextErr()
}

func (s *ShowCommandSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	last1 := time.Date(2015, 3, 20, 0, 0, 0, 0, time.UTC)

	users := []params.ModelUserInfo{{
		UserName:       "admin@local",
		LastConnection: &last1,
		Access:         "write",
	}, {
		UserName:    "bob@local",
		DisplayName: "Bob",
		Access:      "read",
	}}

	s.fake.ResetCalls()
	s.fake.err = nil
	s.fake.info = params.ModelInfo{
		Name:           "mymodel",
		UUID:           testing.ModelTag.Id(),
		ControllerUUID: "1ca2293b-fdb9-4299-97d6-55583bb39364",
		OwnerTag:       "user-admin@local",
		ProviderType:   "openstack",
		Users:          users,
	}

	err := modelcmd.WriteCurrentController("testing")
	c.Assert(err, jc.ErrorIsNil)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		CurrentAccount: "admin@local",
	}
	err = s.store.UpdateModel("testing", "admin@local", "mymodel", jujuclient.ModelDetails{
		testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.store.Models["testing"].AccountModels["admin@local"].CurrentModel = "mymodel"
}

func (s *ShowCommandSuite) TestShow(c *gc.C) {
	_, err := testing.RunCommand(c, model.NewShowCommandForTest(&s.fake, s.store))
	c.Assert(err, jc.ErrorIsNil)
	s.fake.CheckCalls(c, []gitjujutesting.StubCall{
		{"ModelInfo", []interface{}{[]names.ModelTag{testing.ModelTag}}},
		{"Close", nil},
	})
}

func (s *ShowCommandSuite) TestShowFormatYaml(c *gc.C) {
	ctx, err := testing.RunCommand(c, model.NewShowCommandForTest(&s.fake, s.store), "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, `
mymodel:
  model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d
  controller-uuid: 1ca2293b-fdb9-4299-97d6-55583bb39364
  owner: admin@local
  type: openstack
  users:
    admin@local:
      access: write
      last-connection: 2015-03-20
    bob@local:
      display-name: Bob
      access: read
      last-connection: never connected
`[1:])
}

func (s *ShowCommandSuite) TestShowFormatJson(c *gc.C) {
	ctx, err := testing.RunCommand(c, model.NewShowCommandForTest(&s.fake, s.store), "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(ctx), gc.Equals, ""+
		`{"mymodel":{"model-uuid":"deadbeef-0bad-400d-8000-4b1d0d06f00d",`+
		`"controller-uuid":"1ca2293b-fdb9-4299-97d6-55583bb39364",`+
		`"owner":"admin@local","type":"openstack",`+
		`"users":{"admin@local":{"access":"write","last-connection":"2015-03-20"},`+
		`"bob@local":{"display-name":"Bob","access":"read","last-connection":"never connected"}}}}
`)
}

func (s *ShowCommandSuite) TestUnrecognizedArg(c *gc.C) {
	_, err := testing.RunCommand(c, model.NewShowCommandForTest(&s.fake, s.store), "-m", "admin", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}
