// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"bytes"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

func TestCharmTabularSuite(t *stdtesting.T) { tc.Run(t, &CharmTabularSuite{}) }

type CharmTabularSuite struct {
	testhelpers.IsolationSuite
}

func (s *CharmTabularSuite) formatTabular(c *tc.C, value interface{}) string {
	out := &bytes.Buffer{}
	err := resourcecmd.FormatCharmTabular(out, value)
	c.Assert(err, tc.ErrorIsNil)
	return out.String()
}

func (s *CharmTabularSuite) TestFormatCharmTabularOkay(c *tc.C) {
	res := charmRes(c, "spam", ".tgz", "...", "")
	formatted := []resourcecmd.FormattedCharmResource{resourcecmd.FormatCharmResource(res)}

	data := s.formatTabular(c, formatted)
	c.Check(data, tc.Equals, `
Resource  Revision
spam      1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularMinimal(c *tc.C) {
	res := charmRes(c, "spam", "", "", "")
	formatted := []resourcecmd.FormattedCharmResource{resourcecmd.FormatCharmResource(res)}

	data := s.formatTabular(c, formatted)
	c.Check(data, tc.Equals, `
Resource  Revision
spam      1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularUpload(c *tc.C) {
	res := charmRes(c, "spam", "", "", "")
	res.Origin = charmresource.OriginUpload
	formatted := []resourcecmd.FormattedCharmResource{resourcecmd.FormatCharmResource(res)}

	data := s.formatTabular(c, formatted)
	c.Check(data, tc.Equals, `
Resource  Revision
spam      1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularMulti(c *tc.C) {
	formatted := []resourcecmd.FormattedCharmResource{
		resourcecmd.FormatCharmResource(charmRes(c, "spam", ".tgz", "spamspamspamspam", "")),
		resourcecmd.FormatCharmResource(charmRes(c, "eggs", "", "...", "")),
		resourcecmd.FormatCharmResource(charmRes(c, "somethingbig", ".zip", "", "")),
		resourcecmd.FormatCharmResource(charmRes(c, "song", ".mp3", "your favorite", "")),
		resourcecmd.FormatCharmResource(charmRes(c, "avatar", ".png", "your picture", "")),
	}
	formatted[1].Revision = 2

	data := s.formatTabular(c, formatted)
	c.Check(data, tc.Equals, `
Resource      Revision
avatar        1
eggs          2
somethingbig  1
song          1
spam          1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularBadValue(c *tc.C) {
	bogus := "should have been something else"
	err := resourcecmd.FormatCharmTabular(nil, bogus)
	c.Check(err, tc.ErrorMatches, `expected value of type .*`)
}
func TestAppTabularSuite(t *stdtesting.T) { tc.Run(t, &AppTabularSuite{}) }

type AppTabularSuite struct {
	testhelpers.IsolationSuite
}

func (s *AppTabularSuite) formatTabular(c *tc.C, value interface{}) string {
	out := &bytes.Buffer{}
	err := resourcecmd.FormatAppTabular(out, value)
	c.Assert(err, tc.ErrorIsNil)
	return out.String()
}

func (s *AppTabularSuite) TestFormatApplicationOkay(c *tc.C) {
	res := resource.Resource{

		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "openjdk",
				Description: "the java runtime",
			},
			Origin:   charmresource.OriginStore,
			Revision: 7,
		},
		Timestamp: time.Now(),
	}

	formatted := resourcecmd.FormattedApplicationInfo{
		Resources: []resourcecmd.FormattedAppResource{resourcecmd.FormatAppResource(res)},
	}

	data := s.formatTabular(c, formatted)
	c.Check(data, tc.Equals, `
Resource  Supplied by  Revision
openjdk   store        7
`[1:])
}

func (s *AppTabularSuite) TestFormatUnitOkay(c *tc.C) {
	res := resource.Resource{

		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "openjdk",
				Description: "the java runtime",
			},
			Origin:   charmresource.OriginStore,
			Revision: 7,
		},
		Timestamp: time.Now(),
	}

	formatted := []resourcecmd.FormattedAppResource{
		resourcecmd.FormatAppResource(res),
	}

	data := s.formatTabular(c, formatted)
	c.Check(data, tc.Equals, `
Resource  Revision
openjdk   7
`[1:])
}

func (s *AppTabularSuite) TestFormatSvcTabularMulti(c *tc.C) {
	res := []resource.Resource{
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
					Type:        charmresource.TypeFile,
				},
				Origin: charmresource.OriginUpload,
			},
		},
		{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:        "openjdk2",
					Description: "another java runtime",
				},
				Origin:   charmresource.OriginStore,
				Revision: 8,
			},
			Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		},
		{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name:        "website2",
					Description: "your website data",
				},
				Origin: charmresource.OriginUpload,
			},
			RetrievedBy: "Bill User",
			Timestamp:   time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		},
	}

	charmResources := []charmresource.Resource{
		{
			// This resource has a higher revision than the corresponding one
			// above.
			Meta: charmresource.Meta{
				Name:        "openjdk",
				Description: "the java runtime",
			},
			Revision: 10,
			Origin:   charmresource.OriginStore,
		},
		{
			// This resource is the same revision as the corresponding one
			// above.
			Meta: charmresource.Meta{
				Name:        "openjdk2",
				Description: "your website data",
				Type:        charmresource.TypeFile,
				Path:        "foobar",
			},
			Revision: 8,
			Origin:   charmresource.OriginStore,
		},
		{
			// This resource has been overridden by an uploaded resource above,
			// so we won't show it as an available update.
			Meta: charmresource.Meta{
				Name:        "website2",
				Description: "your website data",
			},
			Revision: 99,
			Origin:   charmresource.OriginStore,
		},
		{
			Meta: charmresource.Meta{
				Name:        "website",
				Description: "your website data",
				Type:        charmresource.TypeFile,
			},
		},
	}

	formatted, err := resourcecmd.FormatApplicationResources(resource.ApplicationResources{
		Resources:           res,
		RepositoryResources: charmResources,
	})
	c.Assert(err, tc.ErrorIsNil)

	data := s.formatTabular(c, formatted)
	// Notes: sorted by name, then by revision, newest first.
	c.Check(data, tc.Equals, `
Resource  Supplied by  Revision
openjdk   store        7
openjdk2  store        8
website   upload       -
website2  Bill User    2012-12-12T12:12

[Updates Available]
Resource  Revision
openjdk   10
`[1:])
}

func (s *AppTabularSuite) TestFormatSvcTabularBadValue(c *tc.C) {
	bogus := "should have been something else"
	err := resourcecmd.FormatAppTabular(nil, bogus)
	c.Check(err, tc.ErrorMatches, `unexpected type for data: string`)
}

func (s *AppTabularSuite) TestFormatApplicationDetailsOkay(c *tc.C) {
	res := charmRes(c, "spam", ".tgz", "...", "")
	updates := []resourcecmd.FormattedCharmResource{resourcecmd.FormatCharmResource(res)}

	data := resourcecmd.FormattedApplicationDetails{
		Resources: []resourcecmd.FormattedDetailResource{
			{
				UnitID:      "svc/10",
				UnitNumber:  10,
				Unit:        fakeFmtSvcRes("data", "1"),
				Expected:    fakeFmtSvcRes("data", "1"),
				Progress:    17,
				RevProgress: "combRev1 (fetching: 17%)",
			},
			{
				UnitID:      "svc/5",
				UnitNumber:  5,
				Unit:        fakeFmtSvcRes("config", "2"),
				Expected:    fakeFmtSvcRes("config", "3"),
				RevProgress: "combRev3",
			},
		},
		Updates: updates,
	}

	output := s.formatTabular(c, data)
	c.Assert(output, tc.Equals, `
Unit    Resource  Revision  Expected
svc/5   config    combRev2  combRev3
svc/10  data      combRev1  combRev1 (fetching: 17%)

[Updates Available]
Resource  Revision
spam      1
`[1:])
}

func (s *AppTabularSuite) TestFormatUnitDetailsOkay(c *tc.C) {
	data := resourcecmd.FormattedUnitDetails{
		{
			UnitID:      "svc/10",
			UnitNumber:  10,
			Unit:        fakeFmtSvcRes("data", "1"),
			Expected:    fakeFmtSvcRes("data", "1"),
			RevProgress: "combRev1",
		},
		{
			UnitID:      "svc/10",
			UnitNumber:  10,
			Unit:        fakeFmtSvcRes("config", "2"),
			Expected:    fakeFmtSvcRes("config", "3"),
			Progress:    91,
			RevProgress: "combRev3 (fetching: 91%)",
		},
	}

	output := s.formatTabular(c, data)
	c.Assert(output, tc.Equals, `
Resource  Revision  Expected
config    combRev2  combRev3 (fetching: 91%)
data      combRev1  combRev1
`[1:])
}

func fakeFmtSvcRes(name, suffix string) resourcecmd.FormattedAppResource {
	return resourcecmd.FormattedAppResource{
		ID:               "ID" + suffix,
		ApplicationID:    "svc",
		Name:             name,
		Type:             "Type" + suffix,
		Path:             "Path + suffix",
		Description:      "Desc" + suffix,
		Revision:         "1",
		Fingerprint:      "Fingerprint" + suffix,
		Size:             100,
		Origin:           "Origin" + suffix,
		Used:             true,
		Timestamp:        time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		Username:         "Username" + suffix,
		CombinedRevision: "combRev" + suffix,
		UsedYesNo:        "usedYesNo" + suffix,
		CombinedOrigin:   "combOrig" + suffix,
	}
}
