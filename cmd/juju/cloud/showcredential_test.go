// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/cloud"
	_ "github.com/juju/juju/provider/all"
	coretesting "github.com/juju/juju/testing"
)

type ShowCredentialSuite struct {
	coretesting.BaseSuite

	api *fakeCredentialContentAPI
}

var _ = gc.Suite(&ShowCredentialSuite{})

func (s *ShowCredentialSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.api = &fakeCredentialContentAPI{v: 2}
}

func (s *ShowCredentialSuite) TestShowCredentialBadArgs(c *gc.C) {
	cmd := cloud.NewShowCredentialCommandForTest(s.api)
	_, err := cmdtesting.RunCommand(c, cmd, "cloud")
	c.Assert(err, gc.ErrorMatches, "both cloud and credential name are needed")
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "credential", "extra")
	c.Assert(err, gc.ErrorMatches, `only cloud and credential names are supported`)
}

func (s *ShowCredentialSuite) TestShowCredentialAPIVersion(c *gc.C) {
	s.api.v = 1
	cmd := cloud.NewShowCredentialCommandForTest(s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Controller does not support credential content lookup\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
	s.api.CheckCallNames(c, "BestAPIVersion", "Close")
}

func (s *ShowCredentialSuite) TestShowCredentialAPICallError(c *gc.C) {
	s.api.SetErrors(errors.New("boom"), nil)
	cmd := cloud.NewShowCredentialCommandForTest(s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Getting credential content failed with: boom\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
	s.api.CheckCallNames(c, "BestAPIVersion", "CredentialContents", "Close")
}

func (s *ShowCredentialSuite) TestShowCredentialNone(c *gc.C) {
	s.api.contents = []params.CredentialContentResult{}
	cmd := cloud.NewShowCredentialCommandForTest(s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "No credential to display\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ``)
	s.api.CheckCallNames(c, "BestAPIVersion", "CredentialContents", "Close")
}

func (s *ShowCredentialSuite) TestShowCredentialOne(c *gc.C) {
	s.api.contents = []params.CredentialContentResult{
		{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:      "cloud-name",
					Name:       "credential-name",
					AuthType:   "userpass",
					Attributes: map[string]string{"username": "fred"},
					Secrets:    map[string]string{"password": "sekret"},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
					{Model: "no-access-model"},
				},
			},
		},
	}
	cmd := cloud.NewShowCredentialCommandForTest(s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, ``)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
credential-name:
  auth-type: userpass
  cloud: cloud-name
  attributes:
    username: fred
  secrets:
    password: sekret
  models:
    abcmodel: admin
    no-access-model: '-'
    xyzmodel: read
`[1:])
	s.api.CheckCallNames(c, "BestAPIVersion", "CredentialContents", "Close")
}

func (s *ShowCredentialSuite) TestShowCredentialMany(c *gc.C) {
	s.api.contents = []params.CredentialContentResult{
		{
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:      "cloud-name",
					Name:       "one",
					AuthType:   "userpass",
					Attributes: map[string]string{"username": "fred"},
					// Don't have secrets here.
				},
				// Don't have models here.
			},
		}, {
			Error: common.ServerError(errors.New("boom")),
		}, {
			Result: &params.ControllerCredentialInfo{
				Content: params.CredentialContent{
					Cloud:    "cloud-name",
					Name:     "two",
					AuthType: "userpass",
					Attributes: map[string]string{
						"username":  "fred",
						"something": "visible-attr",
					},
					Secrets: map[string]string{
						"password": "sekret",
						"hidden":   "very-very-sekret",
					},
				},
				Models: []params.ModelAccess{
					{Model: "abcmodel", Access: "admin"},
					{Model: "xyzmodel", Access: "read"},
					{Model: "no-access-model"},
				},
			},
		},
	}
	cmd := cloud.NewShowCredentialCommandForTest(s.api)
	ctx, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "boom\n")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
one:
  auth-type: userpass
  cloud: cloud-name
  attributes:
    username: fred
two:
  auth-type: userpass
  cloud: cloud-name
  attributes:
    something: visible-attr
    username: fred
  secrets:
    hidden: very-very-sekret
    password: sekret
  models:
    abcmodel: admin
    no-access-model: '-'
    xyzmodel: read
`[1:])
	s.api.CheckCallNames(c, "BestAPIVersion", "CredentialContents", "Close")
}

type fakeCredentialContentAPI struct {
	testing.Stub
	v        int
	contents []params.CredentialContentResult
}

func (f *fakeCredentialContentAPI) CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
	f.AddCall("CredentialContents", cloud, credential, withSecrets)
	return f.contents, f.NextErr()
}

func (f *fakeCredentialContentAPI) Close() error {
	f.AddCall("Close")
	return f.NextErr()
}

func (f *fakeCredentialContentAPI) BestAPIVersion() int {
	f.AddCall("BestAPIVersion")
	return f.v
}
