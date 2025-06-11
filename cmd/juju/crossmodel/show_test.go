// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"context"
	"os"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

func newShowEndpointsCommandForTest(store jujuclient.ClientStore, api ShowAPI) cmd.Command {
	aCmd := &showCommand{newAPIFunc: func(ctx context.Context, controllerName string) (ShowAPI, error) {
		return api, nil
	}}
	aCmd.SetClientStore(store)
	return modelcmd.Wrap(aCmd)
}

type showSuite struct {
	BaseCrossModelSuite
	mockAPI *mockShowAPI
}

func TestShowSuite(t *testing.T) {
	tc.Run(t, &showSuite{})
}

func (s *showSuite) SetUpTest(c *tc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{
		desc:     "IBM DB2 Express Server Edition is an entry level database system",
		offerURL: "prod/model.db2",
	}
}

func (s *showSuite) runShow(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, newShowEndpointsCommandForTest(s.store, s.mockAPI), args...)
}

func (s *showSuite) TestShowNoUrl(c *tc.C) {
	s.assertShowError(c, nil, ".*must specify endpoint URL.*")
}

func (s *showSuite) TestShowApiError(c *tc.C) {
	s.mockAPI.msg = "fail"
	s.assertShowError(c, []string{"prod/model.db2"}, ".*fail.*")
}

func (s *showSuite) TestShowURLError(c *tc.C) {
	s.assertShowError(c, []string{"prod/model.foo/db2"}, "application offer URL has invalid form.*")
}

func (s *showSuite) TestShowWrongModelError(c *tc.C) {
	s.assertShowError(c, []string{"db2"}, `application offer "prod/test.db2" not found`)
}

func (s *showSuite) TestShowNameOnly(c *tc.C) {
	// CurrentModel is prod/test, so ensure api believes offer is in this model
	s.mockAPI.offerURL = "prod/test.db2"
	s.assertShowYaml(c, "db2")
}

func (s *showSuite) TestShowNameAndEnvvarOnly(c *tc.C) {
	// Ensure envvar (prod/model) overrides CurrentModel (prod/test)
	os.Setenv(osenv.JujuModelEnvKey, "prod/model")
	defer func() { _ = os.Unsetenv(osenv.JujuModelEnvKey) }()
	s.assertShowYaml(c, "db2")
}

func (s *showSuite) TestShowYaml(c *tc.C) {
	s.assertShowYaml(c, "prod/model.db2")
}

func (s *showSuite) assertShowYaml(c *tc.C, arg string) {
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

func (s *showSuite) TestShowTabular(c *tc.C) {
	s.assertShow(
		c,
		[]string{"prod/model.db2", "--format", "tabular"},
		`
Store        URL             Access   Description                                 Endpoint  Interface  Role
test-master  prod/model.db2  consume  IBM DB2 Express Server Edition is an entry  db2       http       requirer
                                      level database system                       log       http       provider
`[1:],
	)
}

func (s *showSuite) TestShowDifferentController(c *tc.C) {
	s.mockAPI.controllerName = "different"
	s.assertShow(
		c,
		[]string{"different:prod/model.db2", "--format", "tabular"},
		`
Store      URL             Access   Description                                 Endpoint  Interface  Role
different  prod/model.db2  consume  IBM DB2 Express Server Edition is an entry  db2       http       requirer
                                    level database system                       log       http       provider
`[1:],
	)
}

func (s *showSuite) TestShowTabularExactly180Desc(c *tc.C) {
	s.mockAPI.desc = s.mockAPI.desc + s.mockAPI.desc + s.mockAPI.desc[:52]
	s.assertShow(
		c,
		[]string{"prod/model.db2", "--format", "tabular"},
		`
Store        URL             Access   Description                                   Endpoint  Interface  Role
test-master  prod/model.db2  consume  IBM DB2 Express Server Edition is an entry    db2       http       requirer
                                      level database systemIBM DB2 Express Server   log       http       provider
                                      Edition is an entry level database systemIBM                       
                                      DB2 Express Server Edition is an entry level                       
                                      dat                                                                
`[1:],
	)
}

func (s *showSuite) TestShowTabularMoreThan180Desc(c *tc.C) {
	s.mockAPI.desc = s.mockAPI.desc + s.mockAPI.desc + s.mockAPI.desc
	s.assertShow(
		c,
		[]string{"prod/model.db2", "--format", "tabular"},
		`
Store        URL             Access   Description                                   Endpoint  Interface  Role
test-master  prod/model.db2  consume  IBM DB2 Express Server Edition is an entry    db2       http       requirer
                                      level database systemIBM DB2 Express Server   log       http       provider
                                      Edition is an entry level database systemIBM                       
                                      DB2 Express Server Edition is an entry level                       
                                      ...                                                                
`[1:],
	)
}

func (s *showSuite) assertShow(c *tc.C, args []string, expected string) {
	context, err := s.runShow(c, args...)
	c.Assert(err, tc.ErrorIsNil)

	obtained := cmdtesting.Stdout(context)
	c.Assert(obtained, tc.Matches, expected)
}

func (s *showSuite) assertShowError(c *tc.C, args []string, expected string) {
	_, err := s.runShow(c, args...)
	c.Assert(err, tc.ErrorMatches, expected)
}

type mockShowAPI struct {
	controllerName string
	offerURL       string
	msg, desc      string
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) ApplicationOffer(ctx context.Context, url string) (*jujucrossmodel.ApplicationOfferDetails, error) {
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
