// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"time"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
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
		client: &stubServiceClient{stub: stub},
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
	c.Assert(s.target, gc.Equals, "foo")
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
		Name:    "list-resources",
		Aliases: []string{"resources"},
		Args:    "service-or-unit",
		Purpose: "show the resources for a service or unit",
		Doc: `
This command shows the resources required by and those in use by an existing
service or unit in your model.  When run for a service, it will also show any
updates available for resources from the charmstore.
`,
	})
}

func (s *ShowServiceSuite) TestRun(c *gc.C) {
	data := []resource.ServiceResources{
		{
			Resources: []resource.Resource{
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "openjdk",
							Description: "the java runtime",
						},
						Origin:   charmresource.OriginStore,
						Revision: 7,
					},
				},
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "website",
							Description: "your website data",
						},
						Origin: charmresource.OriginUpload,
					},
				},
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "rsc1234",
							Description: "a big description",
						},
						Origin:   charmresource.OriginStore,
						Revision: 15,
					},
					Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
				},
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "website2",
							Description: "awesome data",
						},
						Origin: charmresource.OriginUpload,
					},
					Username:  "Bill User",
					Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
				},
			},
			CharmStoreResources: []charmresource.Resource{
				{
					// This resource has a higher revision than the corresponding one
					// above.
					Meta: charmresource.Meta{
						Name:        "openjdk",
						Description: "the java runtime",
						Type:        charmresource.TypeFile,
						Path:        "foobar",
					},
					Revision: 10,
					Origin:   charmresource.OriginStore,
				},
				{
					// This resource is the same revision as the corresponding one
					// above.
					Meta: charmresource.Meta{
						Name:        "rsc1234",
						Description: "a big description",
						Type:        charmresource.TypeFile,
						Path:        "foobar",
					},
					Revision: 15,
					Origin:   charmresource.OriginStore,
				},
				{
					Meta: charmresource.Meta{
						Name:        "website",
						Description: "your website data",
					},
					Origin: charmresource.OriginUpload,
				},
				{
					Meta: charmresource.Meta{
						Name:        "website2",
						Description: "awesome data",
					},
					Origin: charmresource.OriginUpload,
				},
			},
		},
	}
	s.stubDeps.client.ReturnResources = data

	cmd := &ShowServiceCommand{
		deps: ShowServiceDeps{
			NewClient: s.stubDeps.NewClient,
		},
	}

	code, stdout, stderr := runCmd(c, cmd, "svc")
	c.Check(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
[Service]
RESOURCE SUPPLIED BY REVISION
openjdk  charmstore  7
website  upload      -
rsc1234  charmstore  15
website2 Bill User   2012-12-12T12:12

[Updates Available]
RESOURCE REVISION
openjdk  10

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowServiceSuite) TestRunUnit(c *gc.C) {
	data := []resource.ServiceResources{{
		UnitResources: []resource.UnitResources{{
			Tag: names.NewUnitTag("svc/0"),
			Resources: []resource.Resource{
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "rsc1234",
							Description: "a big description",
						},
						Origin:   charmresource.OriginStore,
						Revision: 15,
					},
					Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
				},
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "website2",
							Description: "awesome data",
						},
						Origin: charmresource.OriginUpload,
					},
					Username:  "Bill User",
					Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
				},
			},
		}},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := &ShowServiceCommand{
		deps: ShowServiceDeps{
			NewClient: s.stubDeps.NewClient,
		},
	}

	code, stdout, stderr := runCmd(c, cmd, "svc/0")
	c.Assert(code, gc.Equals, 0)
	c.Assert(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
[Unit]
RESOURCE REVISION
rsc1234  15
website2 2012-12-12T12:12

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowServiceSuite) TestRunDetails(c *gc.C) {
	data := []resource.ServiceResources{{
		Resources: []resource.Resource{
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "alpha",
						Description: "a big comment",
					},
					Origin:   charmresource.OriginStore,
					Revision: 15,
				},
				Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "charlie",
						Description: "awesome data",
					},
					Origin: charmresource.OriginUpload,
				},
				Username:  "Bill User",
				Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "beta",
						Description: "more data",
					},
					Origin: charmresource.OriginUpload,
				},
				Username:  "Bill User",
				Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
		},
		CharmStoreResources: []charmresource.Resource{
			{
				Meta: charmresource.Meta{
					Name:        "alpha",
					Description: "a big comment",
				},
				Origin:   charmresource.OriginStore,
				Revision: 15,
			},
			{
				Meta: charmresource.Meta{
					Name:        "charlie",
					Description: "awesome data",
				},
				Origin: charmresource.OriginUpload,
			},
			{
				Meta: charmresource.Meta{
					Name:        "beta",
					Description: "more data",
				},
				Origin: charmresource.OriginUpload,
			},
		},
		UnitResources: []resource.UnitResources{
			{
				Tag: names.NewUnitTag("svc/10"),
				Resources: []resource.Resource{
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "alpha",
								Description: "a big comment",
							},
							Origin:   charmresource.OriginStore,
							Revision: 10, // note the reivision is different for this unit
						},
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "charlie",
								Description: "awesome data",
							},
							Origin: charmresource.OriginUpload,
						},
						Username: "Bill User",
						// note the different time
						Timestamp: time.Date(2011, 11, 11, 11, 11, 11, 0, time.UTC),
					},
					// note we're missing the beta resource for this unit
				},
			},
			{
				Tag: names.NewUnitTag("svc/5"),
				Resources: []resource.Resource{
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "alpha",
								Description: "a big comment",
							},
							Origin:   charmresource.OriginStore,
							Revision: 10, // note the reivision is different for this unit
						},
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "charlie",
								Description: "awesome data",
							},
							Origin: charmresource.OriginUpload,
						},
						Username: "Bill User",
						// note the different time
						Timestamp: time.Date(2011, 11, 11, 11, 11, 11, 0, time.UTC),
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "beta",
								Description: "more data",
							},
							Origin: charmresource.OriginUpload,
						},
						Username:  "Bill User",
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
				},
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := &ShowServiceCommand{
		deps: ShowServiceDeps{
			NewClient: s.stubDeps.NewClient,
		},
	}

	code, stdout, stderr := runCmd(c, cmd, "svc", "--details")
	c.Check(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
[Units]
UNIT RESOURCE REVISION         EXPECTED
5    alpha    10               15
5    beta     2012-12-12T12:12 2012-12-12T12:12
5    charlie  2011-11-11T11:11 2012-12-12T12:12
10   alpha    10               15
10   beta     -                2012-12-12T12:12
10   charlie  2011-11-11T11:11 2012-12-12T12:12

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowServiceSuite) TestRunUnitDetails(c *gc.C) {
	data := []resource.ServiceResources{{
		Resources: []resource.Resource{
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "alpha",
						Description: "a big comment",
					},
					Origin:   charmresource.OriginStore,
					Revision: 15,
				},
				Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "charlie",
						Description: "awesome data",
					},
					Origin: charmresource.OriginUpload,
				},
				Username:  "Bill User",
				Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "beta",
						Description: "more data",
					},
					Origin: charmresource.OriginUpload,
				},
				Username:  "Bill User",
				Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
		},
		UnitResources: []resource.UnitResources{
			{
				Tag: names.NewUnitTag("svc/10"),
				Resources: []resource.Resource{
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "alpha",
								Description: "a big comment",
							},
							Origin:   charmresource.OriginStore,
							Revision: 10, // note the reivision is different for this unit
						},
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "charlie",
								Description: "awesome data",
							},
							Origin: charmresource.OriginUpload,
						},
						Username: "Bill User",
						// note the different time
						Timestamp: time.Date(2011, 11, 11, 11, 11, 11, 0, time.UTC),
					},
					// note we're missing the beta resource for this unit
				},
			},
			{
				Tag: names.NewUnitTag("svc/5"),
				Resources: []resource.Resource{
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "alpha",
								Description: "a big comment",
							},
							Origin:   charmresource.OriginStore,
							Revision: 10, // note the reivision is different for this unit
						},
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "charlie",
								Description: "awesome data",
							},
							Origin: charmresource.OriginUpload,
						},
						Username: "Bill User",
						// note the different time
						Timestamp: time.Date(2011, 11, 11, 11, 11, 11, 0, time.UTC),
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "beta",
								Description: "more data",
							},
							Origin: charmresource.OriginUpload,
						},
						Username:  "Bill User",
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
				},
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := &ShowServiceCommand{
		deps: ShowServiceDeps{
			NewClient: s.stubDeps.NewClient,
		},
	}

	code, stdout, stderr := runCmd(c, cmd, "svc/10", "--details")
	c.Assert(code, gc.Equals, 0)
	c.Assert(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
[Unit]
RESOURCE REVISION         EXPECTED
alpha    10               15
beta     -                2012-12-12T12:12
charlie  2011-11-11T11:11 2012-12-12T12:12

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

type stubShowServiceDeps struct {
	stub   *testing.Stub
	client *stubServiceClient
}

func (s *stubShowServiceDeps) NewClient(c *ShowServiceCommand) (ShowServiceClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

type stubServiceClient struct {
	stub            *testing.Stub
	ReturnResources []resource.ServiceResources
}

func (s *stubServiceClient) ListResources(services []string) ([]resource.ServiceResources, error) {
	s.stub.AddCall("ListResources", services)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.ReturnResources, nil
}

func (s *stubServiceClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
