// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"fmt"
	"strings"
	"time"

	charmresource "github.com/juju/charm/v7/resource"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	resourcecmd "github.com/juju/juju/cmd/juju/resource"
	"github.com/juju/juju/resource"
)

var _ = gc.Suite(&CharmFormatterSuite{})

type CharmFormatterSuite struct {
	testing.IsolationSuite
}

func (s *CharmFormatterSuite) TestFormatCharmResource(c *gc.C) {
	res := charmRes(c, "spam", ".tgz", "X", "spamspamspam")
	res.Revision = 5

	formatted := resourcecmd.FormatCharmResource(res)

	c.Check(formatted, jc.DeepEquals, resourcecmd.FormattedCharmResource{
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

var _ = gc.Suite(&SvcFormatterSuite{})

type SvcFormatterSuite struct {
	testing.IsolationSuite
}

func (s *SvcFormatterSuite) TestFormatSvcResource(c *gc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, jc.ErrorIsNil)
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
		Username:      "Bill User",
		Timestamp:     time.Now().Add(-1 * time.Hour * 24 * 365),
		ID:            "a-application/website",
		ApplicationID: "a-application",
	}

	f := resourcecmd.FormatAppResource(r)
	c.Assert(f, gc.Equals, resourcecmd.FormattedAppResource{
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
		Username:         r.Username,
		CombinedRevision: "5",
		UsedYesNo:        "yes",
		CombinedOrigin:   "charmstore",
	})

}

func (s *SvcFormatterSuite) TestNotUsed(c *gc.C) {
	r := resource.Resource{
		Timestamp: time.Time{},
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.Used, jc.IsFalse)
}

func (s *SvcFormatterSuite) TestUsed(c *gc.C) {
	r := resource.Resource{
		Timestamp: time.Now(),
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.Used, jc.IsTrue)
}

func (s *SvcFormatterSuite) TestOriginUploadDeployed(c *gc.C) {
	// represents what we get when we first deploy an application
	r := resource.Resource{
		Resource: charmresource.Resource{
			Origin: charmresource.OriginUpload,
		},
		Username:  "bill",
		Timestamp: time.Now(),
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.CombinedOrigin, gc.Equals, "bill")
}

func (s *SvcFormatterSuite) TestInitialOriginUpload(c *gc.C) {
	r := resource.Resource{
		Resource: charmresource.Resource{
			Origin: charmresource.OriginUpload,
		},
	}
	f := resourcecmd.FormatAppResource(r)
	c.Assert(f.CombinedOrigin, gc.Equals, "upload")
}

var _ = gc.Suite(&DetailFormatterSuite{})

type DetailFormatterSuite struct {
	testing.IsolationSuite
}

func (s *DetailFormatterSuite) TestFormatDetail(c *gc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, jc.ErrorIsNil)

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
		Username:      "Bill User",
		Timestamp:     time.Now().Add(-1 * time.Hour * 24 * 365),
		ID:            "a-application/website",
		ApplicationID: "a-application",
	}

	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("other"))
	c.Assert(err, jc.ErrorIsNil)

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
		Username:      "Bill User",
		Timestamp:     time.Now(),
		ID:            "a-application/website",
		ApplicationID: "a-application",
	}
	tag := names.NewUnitTag("a-application/55")

	d := resourcecmd.FormatDetailResource(tag, svc, unit, 8)
	c.Assert(d, gc.Equals,
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

func (s *DetailFormatterSuite) TestFormatDetailEmpty(c *gc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("something"))
	c.Assert(err, jc.ErrorIsNil)

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
		Username:      "Bill User",
		Timestamp:     time.Now().Add(-1 * time.Hour * 24 * 365),
		ID:            "a-application/website",
		ApplicationID: "a-application",
	}

	unit := resource.Resource{}
	tag := names.NewUnitTag("a-application/55")

	d := resourcecmd.FormatDetailResource(tag, svc, unit, 0)
	c.Assert(d, gc.Equals,
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
