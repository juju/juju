// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"os"

	"github.com/juju/charm/v11"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

func newShowEndpointsCommandForTest(store jujuclient.ClientStore, api ShowAPI) cmd.Command {
	aCmd := &showCommand{newAPIFunc: func(controllerName string) (ShowAPI, error) {
		return api, nil
	}}
	aCmd.SetClientStore(store)
	return modelcmd.Wrap(aCmd)
}

type showSuite struct {
	BaseCrossModelSuite
	mockAPI *mockShowAPI
}

var _ = gc.Suite(&showSuite{})

func (s *showSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{
		desc:     "IBM DB2 Express Server Edition is an entry level database system",
		offerURL: "fred/model.db2",
	}
}

func (s *showSuite) runShow(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, newShowEndpointsCommandForTest(s.store, s.mockAPI), args...)
}

func (s *showSuite) TestShowNoUrl(c *gc.C) {
	s.assertShowError(c, nil, ".*must specify endpoint URL.*")
}

func (s *showSuite) TestShowApiError(c *gc.C) {
	s.mockAPI.msg = "fail"
	s.assertShowError(c, []string{"fred/model.db2"}, ".*fail.*")
}

func (s *showSuite) TestShowURLError(c *gc.C) {
	s.assertShowError(c, []string{"fred/model.foo/db2"}, "application offer URL has invalid form.*")
}

func (s *showSuite) TestShowWrongModelError(c *gc.C) {
	s.assertShowError(c, []string{"db2"}, `application offer "fred/test.db2" not found`)
}

func (s *showSuite) TestShowNameOnly(c *gc.C) {
	// CurrentModel is fred/test, so ensure api believes offer is in this model
	s.mockAPI.offerURL = "fred/test.db2"
	s.assertShowYaml(c, "db2")
}

func (s *showSuite) TestShowNameAndEnvvarOnly(c *gc.C) {
	// Ensure envvar (fred/model) overrides CurrentModel (fred/test)
	os.Setenv(osenv.JujuModelEnvKey, "fred/model")
	defer func() { os.Unsetenv(osenv.JujuModelEnvKey) }()
	s.assertShowYaml(c, "db2")
}

func (s *showSuite) TestShowYaml(c *gc.C) {
	s.assertShowYaml(c, "fred/model.db2")
}

func (s *showSuite) assertShowYaml(c *gc.C, arg string) {
	s.assertShow(
		c,
		[]string{arg, "--format", "yaml"},
		`
test-master:`[1:]+s.mockAPI.offerURL+`:
  description: IBM DB2 Express Server Edition is an entry level database system
  access: consume
  endpoints:
    db2:
      interface: http
      role: requirer
    log:
      interface: http
      role: provider
  users:
    bob:
      display-name: Bob
      access: consume
`,
	)
}

func (s *showSuite) TestShowTabular(c *gc.C) {
	s.assertShow(
		c,
		[]string{"fred/model.db2", "--format", "tabular"},
		`
Store        URL             Access   Description                                 Endpoint  Interface  Role
test-master  fred/model.db2  consume  IBM DB2 Express Server Edition is an entry  db2       http       requirer
                                      level database system                       log       http       provider
`[1:],
	)
}

func (s *showSuite) TestShowDifferentController(c *gc.C) {
	s.mockAPI.controllerName = "different"
	s.assertShow(
		c,
		[]string{"different:fred/model.db2", "--format", "tabular"},
		`
Store      URL             Access   Description                                 Endpoint  Interface  Role
different  fred/model.db2  consume  IBM DB2 Express Server Edition is an entry  db2       http       requirer
                                    level database system                       log       http       provider
`[1:],
	)
}

func (s *showSuite) TestShowTabularExactly180Desc(c *gc.C) {
	s.mockAPI.desc = s.mockAPI.desc + s.mockAPI.desc + s.mockAPI.desc[:52]
	s.assertShow(
		c,
		[]string{"fred/model.db2", "--format", "tabular"},
		`
Store        URL             Access   Description                                   Endpoint  Interface  Role
test-master  fred/model.db2  consume  IBM DB2 Express Server Edition is an entry    db2       http       requirer
                                      level database systemIBM DB2 Express Server   log       http       provider
                                      Edition is an entry level database systemIBM                       
                                      DB2 Express Server Edition is an entry level                       
                                      dat                                                                
`[1:],
	)
}

func (s *showSuite) TestShowTabularMoreThan180Desc(c *gc.C) {
	s.mockAPI.desc = s.mockAPI.desc + s.mockAPI.desc + s.mockAPI.desc
	s.assertShow(
		c,
		[]string{"fred/model.db2", "--format", "tabular"},
		`
Store        URL             Access   Description                                   Endpoint  Interface  Role
test-master  fred/model.db2  consume  IBM DB2 Express Server Edition is an entry    db2       http       requirer
                                      level database systemIBM DB2 Express Server   log       http       provider
                                      Edition is an entry level database systemIBM                       
                                      DB2 Express Server Edition is an entry level                       
                                      ...                                                                
`[1:],
	)
}

func (s *showSuite) assertShow(c *gc.C, args []string, expected string) {
	context, err := s.runShow(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	obtained := cmdtesting.Stdout(context)
	c.Assert(obtained, gc.Matches, expected)
}

func (s *showSuite) assertShowError(c *gc.C, args []string, expected string) {
	_, err := s.runShow(c, args...)
	c.Assert(err, gc.ErrorMatches, expected)
}

type mockShowAPI struct {
	controllerName string
	offerURL       string
	msg, desc      string
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) ApplicationOffer(url string) (*jujucrossmodel.ApplicationOfferDetails, error) {
	if s.msg != "" {
		return nil, errors.New(s.msg)
	}

	offerURL := s.offerURL
	if s.controllerName != "" {
		offerURL = s.controllerName + ":" + offerURL
	}
	if s.offerURL != url {
		return nil, errors.NotFoundf("application offer %q", url)
	}

	return &jujucrossmodel.ApplicationOfferDetails{
		OfferName:              "hosted-db2",
		OfferURL:               offerURL,
		ApplicationDescription: s.desc,
		Endpoints: []charm.Relation{
			{Name: "log", Interface: "http", Role: charm.RoleProvider},
			{Name: "db2", Interface: "http", Role: charm.RoleRequirer},
		},
		Users: []jujucrossmodel.OfferUserDetails{{
			UserName: "bob", DisplayName: "Bob", Access: "consume",
		}},
	}, nil
}
