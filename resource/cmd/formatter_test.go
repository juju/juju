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

var _ = gc.Suite(&CharmFormatterSuite{})

type CharmFormatterSuite struct {
	testing.IsolationSuite
}

func (s *CharmFormatterSuite) TestFormatCharmResource(c *gc.C) {
	data := []byte("spamspamspam")
	fp, err := charmresource.GenerateFingerprint(data)
	c.Assert(err, jc.ErrorIsNil)
	fingerprint := string(fp.Bytes())
	res := charmRes(c, "spam", ".tgz", "X", fingerprint)
	formatted := FormatCharmResource(res)

	c.Check(formatted, jc.DeepEquals, FormattedCharmResource{
		Name:          "spam",
		Type:          "file",
		Path:          "spam.tgz",
		Comment:       "X",
		Revision:      0,
		Fingerprint:   fp.String(),
		Origin:        "upload",
		charmRevision: "-",
	})
}

var _ = gc.Suite(&SvcFormatterSuite{})

type SvcFormatterSuite struct {
	testing.IsolationSuite
}

func (s *SvcFormatterSuite) TestFormatSvcResource(c *gc.C) {
	fp, err := charmresource.GenerateFingerprint([]byte("something"))
	c.Assert(err, jc.ErrorIsNil)
	r := resource.Resource{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name:    "website",
				Comment: "your website data",
				Type:    charmresource.TypeFile,
				Path:    "foobar",
			},
			Revision:    5,
			Origin:      charmresource.OriginStore,
			Fingerprint: fp,
		},
		Username:  "Bill User",
		Timestamp: time.Now().Add(-1 * time.Hour * 24 * 365),
	}

	f := FormatSvcResource(r)
	c.Assert(f, gc.Equals, FormattedSvcResource{
		Name:             r.Name,
		Type:             "file",
		Path:             r.Path,
		Used:             true,
		Revision:         r.Revision,
		Origin:           "store",
		Fingerprint:      fp.String(),
		Comment:          r.Comment,
		Timestamp:        r.Timestamp,
		Username:         r.Username,
		combinedRevision: "5",
		usedYesNo:        "yes",
		combinedOrigin:   "store",
	})

}

func (s *SvcFormatterSuite) TestNotUsed(c *gc.C) {
	r := resource.Resource{
		Timestamp: time.Time{},
	}
	f := FormatSvcResource(r)
	c.Assert(f.Used, jc.IsFalse)
}

func (s *SvcFormatterSuite) TestUsed(c *gc.C) {
	r := resource.Resource{
		Timestamp: time.Now(),
	}
	f := FormatSvcResource(r)
	c.Assert(f.Used, jc.IsTrue)
}
