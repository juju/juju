// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/testhelpers"
	jtesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestFakeAPISuite(t *testing.T) {
	tc.Run(t, &fakeAPISuite{})
}

type fakeAPISuite struct {
	testhelpers.IsolationSuite
}

const fakeUUID = "f47ac10b-58cc-dead-beef-0e02b2c3d479"

func (*fakeAPISuite) TestFakeAPI(c *tc.C) {
	var r root
	srv := apiservertesting.NewAPIServer(func(modelUUID string) (interface{}, error) {
		c.Check(modelUUID, tc.Equals, fakeUUID)
		return &r, nil
	})
	defer srv.Close()
	info := &api.Info{
		Addrs:    srv.Addrs,
		CACert:   jtesting.CACert,
		ModelTag: names.NewModelTag(fakeUUID),
	}
	_, err := api.Open(c.Context(), info, api.DialOpts{})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(r.calledMethods, tc.DeepEquals, []string{"Login"})
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
