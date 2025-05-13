// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/core/resource"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

var _ = tc.Suite(&CharmFormatterSuite{})

type CharmFormatterSuite struct {
	testhelpers.IsolationSuite
}

func (s *CharmFormatterSuite) TestFormatCharmResource(c *tc.C) {
	res := charmRes(c, "spam", ".tgz", "X", "spamspamspam")
	res.Revision = 5

	formatted := resourcecmd.FormatCharmResource(res)

	c.Check(formatted, tc.DeepEquals, resourcecmd.FormattedCharmResource{
		Name:        "spam",
		Type:        "file",
		Path:        "spam.tgz",
		Description: "X",
		Revision:    5,
		Fingerprint: res.Fingerprint.String(),
		Size:        int64(len("spamspamspam")),
		Origin:      "store",
	})
}

var _ = tc.Suite(&SvcFormatterSuite{})

type SvcFormatterSuite struct {
	testhelpers.IsolationSuite
}

func (s *SvcFormatterSuite) TestFormatSvcResource(c *tc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, tc.ErrorIsNil)
	r := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "website",
				Description: "your website data",
				Type:        charmresource.TypeFile,
				Path:        "foobar",
			},
			Revision:    5,
			Origin:      charmresource.OriginStore,
			Fingerprint: fp,
			Size:        10,
		},
		RetrievedBy:     "Bill User",
		Timestamp:       time.Now().Add(-1 * time.Hour * 24 * 365),
		UUID:            "a-application/website",
		ApplicationName: "a-application",
	}

	f := resourcecmd.FormatAppResource(r)
	c.Assert(f, tc.Equals, resourcecmd.FormattedAppResource{
		ID:               "a-application/website",
		ApplicationID:    "a-application",
		Name:             r.Name,
		Type:             "file",
		Path:             r.Path,
		Used:             true,
		Revision:         fmt.Sprintf("%v", r.Revision),
		Origin:           "store",
		Fingerprint:      fp.String(),
		Size:             10,
		Description:      r.Description,
		Timestamp:        r.Timestamp,
		Username:         r.RetrievedBy,
		CombinedRevision: "5",
		UsedYesNo:        "yes",
		CombinedOrigin:   "store",
	})
}

func (s *SvcFormatterSuite) TestFormatSvcResourceUpload(c *tc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, tc.ErrorIsNil)
	r := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "vincent-van-gogh-blog",
				Description: "Vincent van Gogh blog",
				Type:        charmresource.TypeFile,
				Path:        "foobar",
			},
			Origin:      charmresource.OriginUpload,
			Fingerprint: fp,
			Size:        10,
		},
		RetrievedBy:     "vvgogh",
		Timestamp:       time.Now().Add(-1 * time.Hour * 24 * 365),
		UUID:            "a-application/website",
		ApplicationName: "a-application",
	}

	f := resourcecmd.FormatAppResource(r)
	c.Assert(f, tc.Equals, resourcecmd.FormattedAppResource{
		ID:               "a-application/website",
		ApplicationID:    "a-application",
		Name:             r.Name,
		Type:             "file",
		Path:             r.Path,
		Used:             true,
		Revision:         fmt.Sprintf("%v", r.Revision),
		Origin:           "upload",
		Fingerprint:      fp.String(),
		Size:             10,
		Description:      r.Description,
		Timestamp:        r.Timestamp,
		Username:         r.RetrievedBy,
		CombinedRevision: r.Timestamp.Format("2006-01-02T15:04"),
		UsedYesNo:        "yes",
		CombinedOrigin:   "vvgogh",
	})
}

func (s *SvcFormatterSuite) TestNotUsed(c *tc.C) {
	r := resource.Resource{
		Timestamp: time.Time{},
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.Used, tc.IsFalse)
}

func (s *SvcFormatterSuite) TestUsed(c *tc.C) {
	r := resource.Resource{
		Timestamp: time.Now(),
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.Used, tc.IsTrue)
}

func (s *SvcFormatterSuite) TestOriginUploadDeployed(c *tc.C) {
	// represents what we get when we first deploy an application
	r := resource.Resource{
		Resource: charmresource.Resource{
			Origin: charmresource.OriginUpload,
		},
		RetrievedBy: "bill",
		Timestamp:   time.Now(),
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.CombinedOrigin, tc.Equals, "bill")
}

func (s *SvcFormatterSuite) TestInitialOriginUpload(c *tc.C) {
	r := resource.Resource{
		Resource: charmresource.Resource{
			Origin: charmresource.OriginUpload,
		},
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.CombinedOrigin, tc.Equals, "upload")
}

var _ = tc.Suite(&DetailFormatterSuite{})

type DetailFormatterSuite struct {
	testhelpers.IsolationSuite
}

func (s *DetailFormatterSuite) TestFormatDetail(c *tc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, tc.ErrorIsNil)

	svc := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "website",
				Description: "your website data",
				Type:        charmresource.TypeFile,
				Path:        "foobar",
			},
			Revision:    5,
			Origin:      charmresource.OriginStore,
			Fingerprint: fp,
			Size:        10,
		},
		RetrievedBy:     "Bill User",
		Timestamp:       time.Now().Add(-1 * time.Hour * 24 * 365),
		UUID:            "a-application/website",
		ApplicationName: "a-application",
	}

	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("other"))
	c.Assert(err, tc.ErrorIsNil)

	unit := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "website",
				Description: "your website data",
				Type:        charmresource.TypeFile,
				Path:        "foobar",
			},
			Revision:    7,
			Origin:      charmresource.OriginStore,
			Fingerprint: fp2,
			Size:        15,
		},
		RetrievedBy:     "Bill User",
		Timestamp:       time.Now(),
		UUID:            "a-application/website",
		ApplicationName: "a-application",
	}
	tag := names.NewUnitTag("a-application/55")

	d := resourcecmd.FormatDetailResource(tag, svc, unit, 8)
	c.Assert(d, tc.Equals,
		resourcecmd.FormattedDetailResource{
			UnitNumber:  55,
			UnitID:      "a-application/55",
			Expected:    resourcecmd.FormatAppResource(svc),
			Progress:    8,
			RevProgress: "5 (fetching: 80%)",
			Unit:        resourcecmd.FormatAppResource(unit),
		},
	)
}

func (s *DetailFormatterSuite) TestFormatDetailEmpty(c *tc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, tc.ErrorIsNil)

	svc := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:        "website",
				Description: "your website data",
				Type:        charmresource.TypeFile,
				Path:        "foobar",
			},
			Revision:    5,
			Origin:      charmresource.OriginStore,
			Fingerprint: fp,
			Size:        10,
		},
		RetrievedBy:     "Bill User",
		Timestamp:       time.Now().Add(-1 * time.Hour * 24 * 365),
		UUID:            "a-application/website",
		ApplicationName: "a-application",
	}

	unit := resource.Resource{}
	tag := names.NewUnitTag("a-application/55")

	d := resourcecmd.FormatDetailResource(tag, svc, unit, 0)
	c.Assert(d, tc.Equals,
		resourcecmd.FormattedDetailResource{
			UnitNumber:  55,
			UnitID:      "a-application/55",
			Expected:    resourcecmd.FormatAppResource(svc),
			Progress:    0,
			RevProgress: "5 (fetching: 0%)",
			Unit:        resourcecmd.FormatAppResource(unit),
		},
	)
}
