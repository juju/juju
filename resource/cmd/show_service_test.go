// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"time"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&ShowServiceSuite{})

type ShowServiceSuite struct {
	testing.IsolationSuite

	stubDeps *stubShowServiceDeps
}

func (s *ShowServiceSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	stub := &testing.Stub{}
	s.stubDeps = &stubShowServiceDeps{
		stub:   stub,
		client: &stubCharmStore{stub: stub},
	}
}

func (*ShowServiceSuite) TestInitEmpty(c *gc.C) {
	s := ShowServiceCommand{}

	err := s.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*ShowServiceSuite) TestInitGood(c *gc.C) {
	s := ShowServiceCommand{}
	err := s.Init([]string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.service, gc.Equals, "foo")
}

func (*ShowServiceSuite) TestInitTooManyArgs(c *gc.C) {
	s := ShowServiceCommand{}

	err := s.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *ShowServiceSuite) TestInfo(c *gc.C) {
	var command ShowServiceCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "resources",
		Args:    "service",
		Purpose: "show the resources for a service",
		Doc: `
This command shows the resources required by and those in use by an existing service in your model.
`,
	})
}

func (s *ShowServiceSuite) TestRun(c *gc.C) {
	data := []resource.Resource{
		{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:    "openjdk",
					Comment: "the java runtime",
				},
				Origin:   charmresource.OriginStore,
				Revision: 7,
			},
		},
		{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:    "website",
					Comment: "your website data",
				},
				Origin: charmresource.OriginUpload,
			},
			Username: "Sandra User",
		},
		{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:    "openjdk",
					Comment: "the java runtime",
				},
				Origin:   charmresource.OriginStore,
				Revision: 8,
			},
			Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		},
		{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:    "website",
					Comment: "your website data",
				},
				Origin: charmresource.OriginUpload,
			},
			Username:  "Bill User",
			Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		},
	}
	s.stubDeps.client.ReturnResources = data

	cmd := &ShowServiceCommand{
		deps: ShowServiceDeps{
			NewClient: s.stubDeps.NewClient,
		},
	}

	code, stdout, stderr := runCmd(c, cmd, "svc")
	c.Assert(code, gc.Equals, 0)
	c.Assert(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
RESOURCE ORIGIN      REV        USED COMMENT
openjdk  store       8          yes  the java runtime
openjdk  store       7          no   the java runtime
website  Bill User   2012-12-12 yes  your website data
website  Sandra User -          no   your website data

`[1:])

	checkCall(c, s.stubDeps.stub, "ShowService", [][]interface{}{{"svc"}})
}

type stubShowServiceDeps struct {
	stub   *testing.Stub
	client *stubCharmStore
}

func (s *stubShowServiceDeps) NewClient(c *ShowServiceCommand) (ShowServiceClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}
