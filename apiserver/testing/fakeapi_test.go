// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	jtesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

var _ = gc.Suite(&fakeAPISuite{})

type fakeAPISuite struct {
	testing.IsolationSuite
}

const fakeUUID = "f47ac10b-58cc-dead-beef-0e02b2c3d479"

func (*fakeAPISuite) TestFakeAPI(c *gc.C) {
	var r root
	srv := apiservertesting.NewAPIServer(func(modelUUID string) interface{} {
		c.Check(modelUUID, gc.Equals, fakeUUID)
		return &r
	})
	defer srv.Close()
	info := &api.Info{
		Addrs:    srv.Addrs,
		CACert:   jtesting.CACert,
		ModelTag: names.NewModelTag(fakeUUID),
	}
	_, err := api.Open(info, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(r.calledMethods, jc.DeepEquals, []string{"Login"})
}

type root struct {
	calledMethods []string
}

type facade struct {
	r *root
}

func (r *root) Admin(id string) (facade, error) {
	return facade{r}, nil
}

func (f facade) Login(req params.LoginRequest) (params.LoginResult, error) {
	f.r.calledMethods = append(f.r.calledMethods, "Login")
	return params.LoginResult{
		ModelTag:      names.NewModelTag(fakeUUID).String(),
		ControllerTag: names.NewControllerTag(fakeUUID).String(),
		UserInfo: &params.AuthUserInfo{
			DisplayName: "foo",
			Identity:    "user-bar",
		},
		ServerVersion: version.Current.String(),
	}, nil
}
