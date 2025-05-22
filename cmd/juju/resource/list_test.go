// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

func TestShowApplicationSuite(t *testing.T) {
	tc.Run(t, &ShowApplicationSuite{})
}

type ShowApplicationSuite struct {
	testhelpers.IsolationSuite

	stubDeps *stubShowApplicationDeps
}

func (s *ShowApplicationSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	stub := &testhelpers.Stub{}
	s.stubDeps = &stubShowApplicationDeps{
		stub:   stub,
		client: &stubResourceClient{stub: stub},
	}
}

func (*ShowApplicationSuite) TestInitEmpty(c *tc.C) {
	s := resourcecmd.NewListCommandForTest(nil)

	err := s.Init([]string{})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (*ShowApplicationSuite) TestInitGood(c *tc.C) {
	s := resourcecmd.NewListCommandForTest(nil)
	err := s.Init([]string{"foo"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resourcecmd.ListCommandTarget(s), tc.Equals, "foo")
}

func (*ShowApplicationSuite) TestInitTooManyArgs(c *tc.C) {
	s := resourcecmd.NewListCommandForTest(nil)

	err := s.Init([]string{"foo", "bar"})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *ShowApplicationSuite) TestInfo(c *tc.C) {
	var command resourcecmd.ListCommand
	info := command.Info()

	// Verify that Info is wired up. Without verifying exact text.
	c.Check(info.Name, tc.Equals, "resources")
	c.Check(info.Args, tc.Not(tc.Equals), "")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
	c.Check(info.Doc, tc.Not(tc.Equals), "")
	c.Check(info.FlagKnownAs, tc.Not(tc.Equals), "")
	c.Check(len(info.ShowSuperFlags), tc.GreaterThan, 2)
}

func (s *ShowApplicationSuite) TestRunNoResourcesForApplication(c *tc.C) {
	data := []resource.ApplicationResources{{}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, "svc")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "No resources to display.\n")
	c.Check(stdout, tc.Equals, "")
	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRun(c *tc.C) {
	data := []resource.ApplicationResources{
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
					RetrievedBy: "Bill User",
					Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
				},
			},
			RepositoryResources: []charmresource.Resource{
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

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, "svc")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")

	c.Check(stdout, tc.Equals, `
Resource  Supplied by  Revision
openjdk   store        7
rsc1234   store        15
website   upload       -
website2  Bill User    2012-12-12T12:12

[Updates Available]
Resource  Revision
openjdk   10
`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunNoResourcesForUnit(c *tc.C) {
	data := []resource.ApplicationResources{{}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, "svc/0")
	c.Assert(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "No resources to display.\n")
	c.Check(stdout, tc.Equals, "")
	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunResourcesForAppButNoResourcesForUnit(c *tc.C) {
	unitName := "svc/0"

	data := []resource.ApplicationResources{{
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
		},
		RepositoryResources: []charmresource.Resource{
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
		},
		UnitResources: []resource.UnitResources{
			{
				Name: coreunit.Name(unitName),
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, unitName)
	c.Assert(code, tc.Equals, 0)
	c.Check(stdout, tc.Equals, `
Resource  Revision
openjdk   -
`[1:])
	c.Check(stderr, tc.Equals, "")
	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunUnit(c *tc.C) {
	data := []resource.ApplicationResources{
		{
			Resources: []resource.Resource{
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "rsc1234",
							Description: "a big description",
						},
						Origin:   charmresource.OriginStore,
						Revision: 20,
					},
					Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					UUID:      "one",
				},
				{
					Resource: charmresource.Resource{
						Meta: charmresource.Meta{
							Name:        "website2",
							Description: "awesome data",
						},
						Origin: charmresource.OriginUpload,
						Size:   15,
					},
					RetrievedBy: "Bill User",
					Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					UUID:        "two",
				},
			},
			UnitResources: []resource.UnitResources{{
				Name: coreunit.Name("svc/0"),
				Resources: []resource.Resource{
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "rsc1234",
								Description: "a big description",
							},
							Origin:   charmresource.OriginStore,
							Revision: 15, // Note revision is different to the application resource
						},
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
						UUID:      "one",
					},
					{
						Resource: charmresource.Resource{
							Meta: charmresource.Meta{
								Name:        "website2",
								Description: "awesome data",
							},
							Origin: charmresource.OriginUpload,
							Size:   15,
						},
						RetrievedBy: "Bill User",
						UUID:        "two",
						Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
				},
			}},
		}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, "svc/0")
	c.Assert(code, tc.Equals, 0)
	c.Assert(stderr, tc.Equals, "")

	c.Check(stdout, tc.Equals, `
Resource  Revision
rsc1234   15
website2  2012-12-12T12:12
`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunDetails(c *tc.C) {
	data := []resource.ApplicationResources{{
		Resources: []resource.Resource{
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "alpha",
						Description: "a big comment",
					},
					Origin:   charmresource.OriginStore,
					Revision: 15,
					Size:     113,
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
					Size:   9835617,
				},
				RetrievedBy: "Bill User",
				Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "beta",
						Description: "more data",
					},
					Origin: charmresource.OriginUpload,
				},
				RetrievedBy: "Bill User",
				Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
		},
		RepositoryResources: []charmresource.Resource{
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
				Name: coreunit.Name("svc/10"),
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
						RetrievedBy: "Bill User",
						// note the different time
						Timestamp: time.Date(2011, 11, 11, 11, 11, 11, 0, time.UTC),
					},
					// note we're missing the beta resource for this unit
				},
			},
			{
				Name: coreunit.Name("svc/5"),
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
						RetrievedBy: "Bill User",
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
						RetrievedBy: "Bill User",
						Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
				},
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, "svc", "--details")
	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")

	c.Check(stdout, tc.Equals, `
Unit    Resource  Revision          Expected
svc/5   alpha     10                15
svc/5   beta      2012-12-12T12:12  2012-12-12T12:12
svc/5   charlie   2011-11-11T11:11  2012-12-12T12:12
svc/10  alpha     10                15
svc/10  beta      -                 2012-12-12T12:12
svc/10  charlie   2011-11-11T11:11  2012-12-12T12:12
`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunUnitDetails(c *tc.C) {
	data := []resource.ApplicationResources{{
		Resources: []resource.Resource{
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "alpha",
						Description: "a big comment",
					},
					Origin:   charmresource.OriginStore,
					Revision: 15,
					Size:     113,
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
					Size:   9835617,
				},
				RetrievedBy: "Bill User",
				Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
			{
				Resource: charmresource.Resource{
					Meta: charmresource.Meta{
						Name:        "beta",
						Description: "more data",
					},
					Origin: charmresource.OriginUpload,
				},
				RetrievedBy: "Bill User",
				Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
			},
		},
		UnitResources: []resource.UnitResources{
			{
				Name: coreunit.Name("svc/10"),
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
						RetrievedBy: "Bill User",
						// note the different time
						Timestamp: time.Date(2011, 11, 11, 11, 11, 11, 0, time.UTC),
					},
					// note we're missing the beta resource for this unit
				},
			},
			{
				Name: coreunit.Name("svc/5"),
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
						RetrievedBy: "Bill User",
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
						RetrievedBy: "Bill User",
						Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
				},
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(s.stubDeps.NewClient)

	code, stdout, stderr := runCmd(c, cmd, "svc/10", "--details")
	c.Assert(code, tc.Equals, 0)
	c.Assert(stderr, tc.Equals, "")

	c.Check(stdout, tc.Equals, `
Resource  Revision          Expected
alpha     10                15
beta      -                 2012-12-12T12:12
charlie   2011-11-11T11:11  2012-12-12T12:12
`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

type stubShowApplicationDeps struct {
	stub   *testhelpers.Stub
	client *stubResourceClient
}

func (s *stubShowApplicationDeps) NewClient(ctx context.Context) (resourcecmd.ListClient, error) {
	s.stub.AddCall("NewClient")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

type stubResourceClient struct {
	stub            *testhelpers.Stub
	ReturnResources []resource.ApplicationResources
}

func (s *stubResourceClient) ListResources(ctx context.Context, applications []string) ([]resource.ApplicationResources, error) {
	s.stub.AddCall("ListResources", applications)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.ReturnResources, nil
}

func (s *stubResourceClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
