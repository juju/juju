// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
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

func (s *CharmTabularSuite) TestFormatCharmTabularOkay(c *gc.C) {
	res := charmRes(c, "spam", ".tgz", "...", "")
	formatted := []FormattedCharmResource{FormatCharmResource(res)}

	data, err := FormatCharmTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE REVISION
spam     1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularMinimal(c *gc.C) {
	res := charmRes(c, "spam", "", "", "")
	formatted := []FormattedCharmResource{FormatCharmResource(res)}

	data, err := FormatCharmTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE REVISION
spam     1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularUpload(c *gc.C) {
	res := charmRes(c, "spam", "", "", "")
	res.Origin = charmresource.OriginUpload
	formatted := []FormattedCharmResource{FormatCharmResource(res)}

	data, err := FormatCharmTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE REVISION
spam     1
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

	data, err := FormatCharmTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
RESOURCE     REVISION
spam         1
eggs         2
somethingbig 1
song         1
avatar       1
`[1:])
}

func (s *CharmTabularSuite) TestFormatCharmTabularBadValue(c *gc.C) {
	bogus := "should have been something else"
	_, err := FormatCharmTabular(bogus)

	c.Check(err, gc.ErrorMatches, `expected value of type .*`)
}

var _ = gc.Suite(&SvcTabularSuite{})

type SvcTabularSuite struct {
	testing.IsolationSuite
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

	data, err := FormatSvcTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
[Service]
RESOURCE SUPPLIED BY REVISION
openjdk  charmstore  7
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

	data, err := FormatSvcTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, `
[Unit]
RESOURCE REVISION
openjdk  7
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

	data, err := FormatSvcTabular(formatted)
	c.Assert(err, jc.ErrorIsNil)

	// Notes: sorted by name, then by revision, newest first.
	c.Check(string(data), gc.Equals, `
[Service]
RESOURCE SUPPLIED BY REVISION
openjdk  charmstore  7
website  upload      -
openjdk2 charmstore  8
website2 Bill User   2012-12-12T12:12

[Updates Available]
RESOURCE REVISION
openjdk  10
`[1:])
}

func (s *SvcTabularSuite) TestFormatSvcTabularBadValue(c *gc.C) {
	bogus := "should have been something else"
	_, err := FormatSvcTabular(bogus)
	c.Check(err, gc.ErrorMatches, `unexpected type for data: string`)
}

var _ = gc.Suite(&DetailsTabularSuite{})

type DetailsTabularSuite struct {
	testing.IsolationSuite
}

func (s *DetailsTabularSuite) TestFormatServiceDetailsOkay(c *gc.C) {
	res := charmRes(c, "spam", ".tgz", "...", "")
	updates := []FormattedCharmResource{FormatCharmResource(res)}

	data := FormattedServiceDetails{
		Resources: []FormattedDetailResource{
			{
				UnitID:     "svc/10",
				unitNumber: 10,
				Unit:       fakeFmtSvcRes("data", "1"),
				Expected:   fakeFmtSvcRes("data", "1"),
			},
			{
				UnitID:     "svc/5",
				unitNumber: 5,
				Unit:       fakeFmtSvcRes("config", "2"),
				Expected:   fakeFmtSvcRes("config", "3"),
			},
		},
		Updates: updates,
	}

	output, err := FormatSvcTabular(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(output), gc.Equals, `
[Units]
UNIT RESOURCE REVISION EXPECTED
5    config   combRev2 combRev3
10   data     combRev1 combRev1

[Updates Available]
RESOURCE REVISION
spam     1
`[1:])
}

func (s *DetailsTabularSuite) TestFormatUnitDetailsOkay(c *gc.C) {
	data := FormattedUnitDetails{
		{
			UnitID:     "svc/10",
			unitNumber: 10,
			Unit:       fakeFmtSvcRes("data", "1"),
			Expected:   fakeFmtSvcRes("data", "1"),
		},
		{
			UnitID:     "svc/10",
			unitNumber: 10,
			Unit:       fakeFmtSvcRes("config", "2"),
			Expected:   fakeFmtSvcRes("config", "3"),
		},
	}

	output, err := FormatSvcTabular(data)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(string(output), gc.Equals, `
[Unit]
RESOURCE REVISION EXPECTED
config   combRev2 combRev3
data     combRev1 combRev1
`[1:])
}

func fakeFmtSvcRes(name, suffix string) FormattedSvcResource {
	return FormattedSvcResource{
		ID:               "ID" + suffix,
		ServiceID:        "svc",
		Name:             name,
		Type:             "Type" + suffix,
		Path:             "Path + suffix",
		Description:      "Desc" + suffix,
		Revision:         1,
		Fingerprint:      "Fingerprint" + suffix,
		Size:             1,
		Origin:           "Origin" + suffix,
		Used:             true,
		Timestamp:        time.Date(2012, 12, 12, 12, 12, 12, 0, time.UTC),
		Username:         "Username" + suffix,
		combinedRevision: "combRev" + suffix,
		usedYesNo:        "usedYesNo" + suffix,
		combinedOrigin:   "combOrig" + suffix,
	}
}
