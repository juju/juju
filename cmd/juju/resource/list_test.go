// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&ShowApplicationSuite{})

type ShowApplicationSuite struct {
	testing.IsolationSuite

	stubDeps *stubShowApplicationDeps
}

func (s *ShowApplicationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	stub := &testing.Stub{}
	s.stubDeps = &stubShowApplicationDeps{
		stub:   stub,
		client: &stubApplicationClient{stub: stub},
	}
}

func (*ShowApplicationSuite) TestInitEmpty(c *gc.C) {
	s := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{})

	err := s.Init([]string{})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (*ShowApplicationSuite) TestInitGood(c *gc.C) {
	s := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{})
	err := s.Init([]string{"foo"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resourcecmd.ListCommandTarget(s), gc.Equals, "foo")
}

func (*ShowApplicationSuite) TestInitTooManyArgs(c *gc.C) {
	s := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{})

	err := s.Init([]string{"foo", "bar"})
	c.Assert(err, jc.Satisfies, errors.IsBadRequest)
}

func (s *ShowApplicationSuite) TestInfo(c *gc.C) {
	var command resourcecmd.ListCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "resources",
		Aliases: []string{"list-resources"},
		Args:    "<application or unit>",
		Purpose: "Show the resources for an application or unit.",
		Doc: `
This command shows the resources required by and those in use by an existing
application or unit in your model.  When run for an application, it will also show any
updates available for resources from the charmstore.
`,
		FlagKnownAs:    "option",
		ShowSuperFlags: []string{"show-log", "debug", "logging-config", "verbose", "quiet", "h", "help"},
	})
}

func (s *ShowApplicationSuite) TestRunNoResourcesForApplication(c *gc.C) {
	data := []resource.ApplicationResources{{}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, "svc")
	c.Check(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "No resources to display.\n")
	c.Check(stdout, gc.Equals, "")
	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRun(c *gc.C) {
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

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, "svc")
	c.Check(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
Resource  Supplied by  Revision
openjdk   charmstore   7
rsc1234   charmstore   15
website   upload       -
website2  Bill User    2012-12-12T12:12

[Updates Available]
Resource  Revision
openjdk   10

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunNoResourcesForUnit(c *gc.C) {
	data := []resource.ApplicationResources{{}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, "svc/0")
	c.Assert(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "No resources to display.\n")
	c.Check(stdout, gc.Equals, "")
	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunResourcesForAppButNoResourcesForUnit(c *gc.C) {
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
		},
		UnitResources: []resource.UnitResources{
			{
				Tag: names.NewUnitTag(unitName),
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, unitName)
	c.Assert(code, gc.Equals, 0)
	c.Check(stdout, gc.Equals, `
Resource  Revision
openjdk   -

`[1:])
	c.Check(stderr, gc.Equals, "")
	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunUnit(c *gc.C) {
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
					ID:        "one",
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
					Username:  "Bill User",
					Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					ID:        "two",
				},
			},
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
							Revision: 15, // Note revision is different to the application resource
						},
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
						ID:        "one",
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
						Username:  "Bill User",
						ID:        "two",
						Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
					},
				},
				DownloadProgress: map[string]int64{
					"website2": 12,
				},
			}},
		}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, "svc/0")
	c.Assert(code, gc.Equals, 0)
	c.Assert(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
Resource  Revision
rsc1234   15
website2  2012-12-12T12:12

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunDetails(c *gc.C) {
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
				DownloadProgress: map[string]int64{
					"alpha":   17,
					"charlie": 899937,
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
				DownloadProgress: map[string]int64{
					"charlie": 177331,
				},
			},
		},
	}}
	s.stubDeps.client.ReturnResources = data

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, "svc", "--details")
	c.Check(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
Unit    Resource  Revision          Expected
svc/5   alpha     10                15
svc/5   beta      2012-12-12T12:12  2012-12-12T12:12
svc/5   charlie   2011-11-11T11:11  2012-12-12T12:12 (fetching: 2%)
svc/10  alpha     10                15 (fetching: 15%)
svc/10  beta      -                 2012-12-12T12:12
svc/10  charlie   2011-11-11T11:11  2012-12-12T12:12 (fetching: 9%)

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

func (s *ShowApplicationSuite) TestRunUnitDetails(c *gc.C) {
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
				DownloadProgress: map[string]int64{
					"charlie": 17,
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

	cmd := resourcecmd.NewListCommandForTest(resourcecmd.ListDeps{
		NewClient: s.stubDeps.NewClient,
	})

	code, stdout, stderr := runCmd(c, cmd, "svc/10", "--details")
	c.Assert(code, gc.Equals, 0)
	c.Assert(stderr, gc.Equals, "")

	c.Check(stdout, gc.Equals, `
Resource  Revision          Expected
alpha     10                15
beta      -                 2012-12-12T12:12
charlie   2011-11-11T11:11  2012-12-12T12:12 (fetching: 0%)

`[1:])

	s.stubDeps.stub.CheckCall(c, 1, "ListResources", []string{"svc"})
}

type stubShowApplicationDeps struct {
	stub   *testing.Stub
	client *stubApplicationClient
}

func (s *stubShowApplicationDeps) NewClient(c *resourcecmd.ListCommand) (resourcecmd.ListClient, error) {
	s.stub.AddCall("NewClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

type stubApplicationClient struct {
	stub            *testing.Stub
	ReturnResources []resource.ApplicationResources
}

func (s *stubApplicationClient) ListResources(applications []string) ([]resource.ApplicationResources, error) {
	s.stub.AddCall("ListResources", applications)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return s.ReturnResources, nil
}

func (s *stubApplicationClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
