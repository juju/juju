// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/crossmodel"
	"github.com/juju/juju/testing"
)

type showSuite struct {
	BaseCrossModelSuite
	mockAPI *mockShowAPI
}

var _ = gc.Suite(&showSuite{})

func (s *showSuite) SetUpTest(c *gc.C) {
	s.BaseCrossModelSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{serviceTag: "service-hosted-db2"}
}

func (s *showSuite) runShow(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, crossmodel.NewShowEndpointsCommandForTest(s.mockAPI), args...)
}

func (s *showSuite) TestShowNoUrl(c *gc.C) {
	s.assertShowError(c, nil, ".*must specify endpoint URL.*")
}

func (s *showSuite) TestShowApiError(c *gc.C) {
	s.mockAPI.msg = "fail"
	s.assertShowError(c, []string{"local:/u/fred/prod/db2"}, ".*fail.*")
}

func (s *showSuite) TestShowConversionError(c *gc.C) {
	s.mockAPI.serviceTag = "invalid_tag"
	s.assertShowError(c, []string{"local:/u/fred/prod/db2"}, ".*could not parse service tag.*")
}

func (s *showSuite) TestShowYaml(c *gc.C) {
	s.assertShow(
		c,
		[]string{"local:/u/fred/prod/db2", "--format", "yaml"},
		`
- service: hosted-db2
  endpoints:
  - db2
  - log
  desc: IBM DB2 Express Server Edition is an entry level database system
`[1:],
	)
}

func (s *showSuite) TestShowTabular(c *gc.C) {
	s.assertShow(
		c,
		[]string{"local:/u/fred/prod/db2", "--format", "tabular"},
		`
SERVICE     INTERFACES  DESCRIPTION
hosted-db2  db2,log     IBM DB2 Express Server Edition is an entry level database system

`[1:],
	)
}

func (s *showSuite) assertShow(c *gc.C, args []string, expected string) {
	context, err := s.runShow(c, args...)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Matches, expected)
}

func (s *showSuite) assertShowError(c *gc.C, args []string, expected string) {
	_, err := s.runShow(c, args...)
	c.Assert(err, gc.ErrorMatches, expected)
}

type mockShowAPI struct {
	msg, serviceTag string
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) Show(url string) (params.EndpointsDetailsResult, error) {
	if s.msg != "" {
		return params.EndpointsDetailsResult{}, errors.New(s.msg)
	}

	return params.EndpointsDetailsResult{
		Service:     s.serviceTag,
		Endpoints:   []string{"db2", "log"},
		Description: "IBM DB2 Express Server Edition is an entry level database system",
	}, nil
}
