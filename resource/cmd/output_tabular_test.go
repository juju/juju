// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"bytes"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&CharmTabularSuite{})

type CharmTabularSuite struct {
	testing.IsolationSuite
}

func (s *CharmTabularSuite) formatTabular(c *gc.C, value interface{}) string {
	out := &bytes.Buffer{}
	err := FormatCharmTabular(out, value)
	c.Assert(err, jc.ErrorIsNil)
	return out.String()
}

func (s *CharmTabularSuite) TestFormatCharmTabularOkay(c *gc.C) {
	res := charmRes(c, "spam", ".tgz", "...", "")
	formatted := []FormattedCharmResource{FormatCharmResource(res)}

	data := s.formatTabular(c, formatted)
	c.Check(data, gc.Equals, `
Resource  Revision
spam      1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularMinimal(c *gc.C) {
	res := charmRes(c, "spam", "", "", "")
	formatted := []FormattedCharmResource{FormatCharmResource(res)}

	data := s.formatTabular(c, formatted)
	c.Check(data, gc.Equals, `
Resource  Revision
spam      1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularUpload(c *gc.C) {
	res := charmRes(c, "spam", "", "", "")
	res.Origin = charmresource.OriginUpload
	formatted := []FormattedCharmResource{FormatCharmResource(res)}

	data := s.formatTabular(c, formatted)
	c.Check(data, gc.Equals, `
Resource  Revision
spam      1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularMulti(c *gc.C) {
	formatted := []FormattedCharmResource{
		FormatCharmResource(charmRes(c, "spam", ".tgz", "spamspamspamspam", "")),
		FormatCharmResource(charmRes(c, "eggs", "", "...", "")),
		FormatCharmResource(charmRes(c, "somethingbig", ".zip", "", "")),
		FormatCharmResource(charmRes(c, "song", ".mp3", "your favorite", "")),
		FormatCharmResource(charmRes(c, "avatar", ".png", "your picture", "")),
	}
	formatted[1].Revision = 2

	data := s.formatTabular(c, formatted)
	c.Check(data, gc.Equals, `
Resource      Revision
spam          1
eggs          2
somethingbig  1
song          1
avatar        1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularBadValue(c *gc.C) {
	bogus := "should have been something else"
	err := FormatCharmTabular(nil, bogus)
	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}

var _ = gc.Suite(&SvcTabularSuite{})

type SvcTabularSuite struct {
	testing.IsolationSuite
}

func (s *SvcTabularSuite) formatTabular(c *gc.C, value interface{}) string {
	out := &bytes.Buffer{}
	err := FormatSvcTabular(out, value)
	c.Assert(err, jc.ErrorIsNil)
	return out.String()
}

func (s *SvcTabularSuite) TestFormatServiceOkay(c *gc.C) {
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

	formatted := FormattedServiceInfo{
		Resources: []FormattedSvcResource{FormatSvcResource(res)},
	}

	data := s.formatTabular(c, formatted)
	c.Check(data, gc.Equals, `
[Service]
Resource  Supplied by  Revision
openjdk   charmstore   7
`[1:])
}

func (s *SvcTabularSuite) TestFormatUnitOkay(c *gc.C) {
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

	formatted := []FormattedUnitResource{
		FormattedUnitResource(FormatSvcResource(res)),
	}

	data := s.formatTabular(c, formatted)
	c.Check(data, gc.Equals, `
[Unit]
Resource  Revision
openjdk   7
`[1:])
}

func (s *SvcTabularSuite) TestFormatSvcTabularMulti(c *gc.C) {
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
			Username:  "Bill User",
			Timestamp: time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
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

	formatted, err := formatServiceResources(resource.ServiceResources{
		Resources:           res,
		CharmStoreResources: charmResources,
	})
	c.Assert(err, jc.ErrorIsNil)

	data := s.formatTabular(c, formatted)
	// Notes: sorted by name, then by revision, newest first.
	c.Check(data, gc.Equals, `
[Service]
Resource  Supplied by  Revision
openjdk   charmstore   7
website   upload       -
openjdk2  charmstore   8
website2  Bill User    2012-12-12T12:12

[Updates Available]
Resource  Revision
openjdk   10
`[1:])
}

func (s *SvcTabularSuite) TestFormatSvcTabularBadValue(c *gc.C) {
	bogus := "should have been something else"
	err := FormatSvcTabular(nil, bogus)
	c.Check(err, gc.ErrorMatches, `unexpected type for data: string`)
}

func (s *SvcTabularSuite) TestFormatServiceDetailsOkay(c *gc.C) {
	res := charmRes(c, "spam", ".tgz", "...", "")
	updates := []FormattedCharmResource{FormatCharmResource(res)}

	data := FormattedServiceDetails{
		Resources: []FormattedDetailResource{
			{
				UnitID:      "svc/10",
				unitNumber:  10,
				Unit:        fakeFmtSvcRes("data", "1"),
				Expected:    fakeFmtSvcRes("data", "1"),
				Progress:    17,
				progress:    "17%",
				revProgress: "combRev1 (fetching: 17%)",
			},
			{
				UnitID:      "svc/5",
				unitNumber:  5,
				Unit:        fakeFmtSvcRes("config", "2"),
				Expected:    fakeFmtSvcRes("config", "3"),
				revProgress: "combRev3",
			},
		},
		Updates: updates,
	}

	output := s.formatTabular(c, data)
	c.Assert(output, gc.Equals, `
[Units]
Unit  Resource  Revision  Expected
5     config    combRev2  combRev3
10    data      combRev1  combRev1 (fetching: 17%)

[Updates Available]
Resource  Revision
spam      1
`[1:])
}

func (s *SvcTabularSuite) TestFormatUnitDetailsOkay(c *gc.C) {
	data := FormattedUnitDetails{
		{
			UnitID:      "svc/10",
			unitNumber:  10,
			Unit:        fakeFmtSvcRes("data", "1"),
			Expected:    fakeFmtSvcRes("data", "1"),
			revProgress: "combRev1",
		},
		{
			UnitID:      "svc/10",
			unitNumber:  10,
			Unit:        fakeFmtSvcRes("config", "2"),
			Expected:    fakeFmtSvcRes("config", "3"),
			Progress:    91,
			progress:    "91%",
			revProgress: "combRev3 (fetching: 91%)",
		},
	}

	output := s.formatTabular(c, data)
	c.Assert(output, gc.Equals, `
[Unit]
Resource  Revision  Expected
config    combRev2  combRev3 (fetching: 91%)
data      combRev1  combRev1
`[1:])
}

func fakeFmtSvcRes(name, suffix string) FormattedSvcResource {
	return FormattedSvcResource{
		ID:               "ID" + suffix,
		ApplicationID:    "svc",
		Name:             name,
		Type:             "Type" + suffix,
		Path:             "Path + suffix",
		Description:      "Desc" + suffix,
		Revision:         1,
		Fingerprint:      "Fingerprint" + suffix,
		Size:             100,
		Origin:           "Origin" + suffix,
		Used:             true,
		Timestamp:        time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		Username:         "Username" + suffix,
		combinedRevision: "combRev" + suffix,
		usedYesNo:        "usedYesNo" + suffix,
		combinedOrigin:   "combOrig" + suffix,
	}
}
