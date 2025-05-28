// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package charm_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charm"
)

type OfferURLSuite struct{}

func TestOfferURLSuite(t *testing.T) {
	tc.Run(t, &OfferURLSuite{})
}

var offerURLTests = []struct {
	s, err string
	exact  string
	url    *charm.OfferURL
}{{
	s:   "controller:qualifier/modelname.applicationname",
	url: &charm.OfferURL{"controller", "qualifier", "modelname", "applicationname"},
}, {
	s:   "controller:qualifier/modelname.applicationname:rel",
	url: &charm.OfferURL{"controller", "qualifier", "modelname", "applicationname:rel"},
}, {
	s:   "modelname.applicationname",
	url: &charm.OfferURL{"", "", "modelname", "applicationname"},
}, {
	s:   "modelname.applicationname:rel",
	url: &charm.OfferURL{"", "", "modelname", "applicationname:rel"},
}, {
	s:   "qualifier/modelname.applicationname:rel",
	url: &charm.OfferURL{"", "qualifier", "modelname", "applicationname:rel"},
}, {
	s:     "/modelname.applicationname",
	url:   &charm.OfferURL{"", "", "modelname", "applicationname"},
	exact: "modelname.applicationname",
}, {
	s:     "/modelname.applicationname:rel",
	url:   &charm.OfferURL{"", "", "modelname", "applicationname:rel"},
	exact: "modelname.applicationname:rel",
}, {
	s:   "qualifier/modelname.applicationname",
	url: &charm.OfferURL{"", "qualifier", "modelname", "applicationname"},
}, {
	s:   "controller:modelname",
	err: `application offer URL is missing application`,
}, {
	s:   "controller:qualifier/modelname",
	err: `application offer URL is missing application`,
}, {
	s:   "model",
	err: `application offer URL is missing application`,
}, {
	s:   "/qualifier/model",
	err: `application offer URL is missing application`,
}, {
	s:   "model.application@bad",
	err: `application name "application@bad" not valid`,
}, {
	s:   "qualifier[bad/model.application",
	err: `qualifier "qualifier\[bad" not valid`,
}, {
	s:   "qualifier/[badmodel.application",
	err: `model name "\[badmodel" not valid`,
}}

func (s *OfferURLSuite) TestParseURL(c *tc.C) {
	for i, t := range offerURLTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := charm.ParseOfferURL(t.s)

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
	url    *charm.OfferURLParts
}{{
	s:   "controller:/qualifier/modelname.applicationname",
	url: &charm.OfferURLParts{"controller", "qualifier", "modelname", "applicationname"},
}, {
	s:   "qualifier/modelname.applicationname",
	url: &charm.OfferURLParts{"", "qualifier", "modelname", "applicationname"},
}, {
	s:   "qualifier/modelname",
	url: &charm.OfferURLParts{"", "qualifier", "modelname", ""},
}, {
	s:   "modelname.application",
	url: &charm.OfferURLParts{"", "", "modelname", "application"},
}, {
	s:   "controller:/modelname",
	url: &charm.OfferURLParts{"controller", "", "modelname", ""},
}, {
	s:   "controller:",
	url: &charm.OfferURLParts{Source: "controller"},
}, {
	s:   "",
	url: &charm.OfferURLParts{},
}, {
	s:   "qualifier/prod/applicationname/extra",
	err: `application offer URL has invalid form, must be \[<qualifier/\]<model>.<appname>: "qualifier/prod/applicationname/extra"`,
}, {
	s:   "controller:/qualifier/modelname.application@bad",
	err: `application name "application@bad" not valid`,
}, {
	s:   "controller:/qualifier[bad/modelname.application",
	err: `qualifier "qualifier\[bad" not valid`,
}, {
	s:   ":foo",
	err: `application offer URL has invalid form, must be \[<qualifier/\]<model>.<appname>: $URL`,
}}

func (s *OfferURLSuite) TestParseURLParts(c *tc.C) {
	for i, t := range urlPartsTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := charm.ParseOfferURLParts(t.s)

		if t.url != nil {
			c.Check(err, tc.IsNil)
			c.Check(url, tc.DeepEquals, t.url)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Assert(err, tc.ErrorMatches, t.err)
			c.Assert(url, tc.IsNil)
		}
	}
}

func (s *OfferURLSuite) TestHasEndpoint(c *tc.C) {
	url, err := charm.ParseOfferURL("model.application:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsTrue)
	url, err = charm.ParseOfferURL("model.application")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsFalse)
	url, err = charm.ParseOfferURL("controller:/qualifier/model.application:thing")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsTrue)
	url, err = charm.ParseOfferURL("controller:/qualifier/model.application")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(url.HasEndpoint(), tc.IsFalse)
}

func (s *OfferURLSuite) TestMakeURL(c *tc.C) {
	url := charm.MakeURL("qualifier", "model", "app", "")
	c.Assert(url, tc.Equals, "qualifier/model.app")
	url = charm.MakeURL("qualifier", "model", "app", "ctrl")
	c.Assert(url, tc.Equals, "ctrl:qualifier/model.app")
}

func (s *OfferURLSuite) TestAsLocal(c *tc.C) {
	url, err := charm.ParseOfferURL("source:model.application:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	expected, err := charm.ParseOfferURL("model.application:endpoint")
	c.Assert(err, tc.ErrorIsNil)
	original := *url
	c.Assert(url.AsLocal(), tc.DeepEquals, expected)
	c.Assert(*url, tc.DeepEquals, original)
}
