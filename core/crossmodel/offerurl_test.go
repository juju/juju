// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/crossmodel"
)

type OfferURLSuite struct{}

func TestOfferURLSuite(t *testing.T) {
	tc.Run(t, &OfferURLSuite{})
}

var urlTests = []struct {
	s, err string
	exact  string
	url    *crossmodel.OfferURL
}{{
	s:   "controller:qualifier/modelname.offername",
	url: &crossmodel.OfferURL{"controller", "qualifier", "modelname", "offername"},
}, {
	s:   "controller:qualifier/modelname.offername:rel",
	url: &crossmodel.OfferURL{"controller", "qualifier", "modelname", "offername:rel"},
}, {
	s:   "modelname.offername",
	url: &crossmodel.OfferURL{"", "", "modelname", "offername"},
}, {
	s:   "modelname.offername:rel",
	url: &crossmodel.OfferURL{"", "", "modelname", "offername:rel"},
}, {
	s:   "qualifier/modelname.offername:rel",
	url: &crossmodel.OfferURL{"", "qualifier", "modelname", "offername:rel"},
}, {
	s:     "/modelname.offername",
	url:   &crossmodel.OfferURL{"", "", "modelname", "offername"},
	exact: "modelname.offername",
}, {
	s:     "/modelname.offername:rel",
	url:   &crossmodel.OfferURL{"", "", "modelname", "offername:rel"},
	exact: "modelname.offername:rel",
}, {
	s:   "qualifier/modelname.offername",
	url: &crossmodel.OfferURL{"", "qualifier", "modelname", "offername"},
}, {
	s:   "controller:modelname",
	err: `offer URL is missing the name`,
}, {
	s:   "controller:qualifier/modelname",
	err: `offer URL is missing the name`,
}, {
	s:   "model",
	err: `offer URL is missing the name`,
}, {
	s:   "/qualifier/model",
	err: `offer URL is missing the name`,
}, {
	s:   "model.offername@bad",
	err: `offer name "offername@bad" not valid`,
}, {
	s:   "qualifier/[badmodel.offername",
	err: `model name "\[badmodel" not valid`,
}}

func (s *OfferURLSuite) TestParseURL(c *tc.C) {
	for i, t := range urlTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseOfferURL(t.s)

		match := t.s
		if t.exact != "" {
			match = t.exact
		}
		if t.url != nil {
			c.Assert(err, tc.IsNil)
			c.Check(url, tc.DeepEquals, t.url)
			c.Check(url.String(), tc.Equals, match)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Check(err, tc.ErrorMatches, t.err)
			c.Check(url, tc.IsNil)
		}
	}
}

var urlPartsTests = []struct {
	s, err string
	url    *crossmodel.OfferURLParts
}{{
	s:   "controller:/qualifier/modelname.offername",
	url: &crossmodel.OfferURLParts{"controller", "qualifier", "modelname", "offername"},
}, {
	s:   "qualifier/modelname.offername",
	url: &crossmodel.OfferURLParts{"", "qualifier", "modelname", "offername"},
}, {
	s:   "qualifier/modelname",
	url: &crossmodel.OfferURLParts{"", "qualifier", "modelname", ""},
}, {
	s:   "modelname.offername",
	url: &crossmodel.OfferURLParts{"", "", "modelname", "offername"},
}, {
	s:   "controller:/modelname",
	url: &crossmodel.OfferURLParts{"controller", "", "modelname", ""},
}, {
	s:   "controller:",
	url: &crossmodel.OfferURLParts{Source: "controller"},
}, {
	s:   "",
	url: &crossmodel.OfferURLParts{},
}, {
	s:   "qualifier/prod/offername/extra",
	err: `offer URL has invalid form, must be .*: "qualifier/prod/offername/extra"`,
}, {
	s:   "controller:/qualifier/modelname.application@bad",
	err: `offer name "application@bad" not valid`,
}, {
	s:   ":foo",
	err: `offer URL has invalid form, must be .*: $URL`,
}}

func (s *OfferURLSuite) TestParseURLParts(c *tc.C) {
	for i, t := range urlPartsTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseOfferURLParts(t.s)

		if t.url != nil {
			c.Check(err, tc.IsNil)
			c.Check(url, tc.DeepEquals, t.url)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Check(err, tc.ErrorMatches, t.err)
			c.Check(url, tc.IsNil)
		}
	}
}

func (s *OfferURLSuite) TestHasEndpoint(c *tc.C) {
	url, err := crossmodel.ParseOfferURL("model.application:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsTrue)
	url, err = crossmodel.ParseOfferURL("model.application")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsFalse)
	url, err = crossmodel.ParseOfferURL("controller:/qualifier/model.application:thing")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsTrue)
	url, err = crossmodel.ParseOfferURL("controller:/qualifier/model.application")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsFalse)
}

func (s *OfferURLSuite) TestMakeURL(c *tc.C) {
	url := crossmodel.MakeURL("qualifier", "model", "app", "")
	c.Assert(url, tc.Equals, "qualifier/model.app")
	url = crossmodel.MakeURL("qualifier", "model", "app", "ctrl")
	c.Assert(url, tc.Equals, "ctrl:qualifier/model.app")
}

func (s *OfferURLSuite) TestAsLocal(c *tc.C) {
	url, err := crossmodel.ParseOfferURL("source:model.application:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	expected, err := crossmodel.ParseOfferURL("model.application:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	original := *url
	c.Assert(url.AsLocal(), tc.DeepEquals, expected)
	c.Assert(*url, tc.DeepEquals, original)
}
