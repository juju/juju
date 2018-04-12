// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"regexp"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/crossmodel"
)

type OfferURLSuite struct{}

var _ = gc.Suite(&OfferURLSuite{})

var urlTests = []struct {
	s, err string
	exact  string
	url    *crossmodel.OfferURL
}{{
	s:   "controller:user/modelname.applicationname",
	url: &crossmodel.OfferURL{"controller", "user", "modelname", "applicationname"},
}, {
	s:   "controller:user/modelname.applicationname:rel",
	url: &crossmodel.OfferURL{"controller", "user", "modelname", "applicationname:rel"},
}, {
	s:   "modelname.applicationname",
	url: &crossmodel.OfferURL{"", "", "modelname", "applicationname"},
}, {
	s:   "modelname.applicationname:rel",
	url: &crossmodel.OfferURL{"", "", "modelname", "applicationname:rel"},
}, {
	s:   "user/modelname.applicationname:rel",
	url: &crossmodel.OfferURL{"", "user", "modelname", "applicationname:rel"},
}, {
	s:     "/modelname.applicationname",
	url:   &crossmodel.OfferURL{"", "", "modelname", "applicationname"},
	exact: "modelname.applicationname",
}, {
	s:     "/modelname.applicationname:rel",
	url:   &crossmodel.OfferURL{"", "", "modelname", "applicationname:rel"},
	exact: "modelname.applicationname:rel",
}, {
	s:   "user/modelname.applicationname",
	url: &crossmodel.OfferURL{"", "user", "modelname", "applicationname"},
}, {
	s:   "controller:modelname",
	err: `application offer URL is missing application`,
}, {
	s:   "controller:user/modelname",
	err: `application offer URL is missing application`,
}, {
	s:   "model",
	err: `application offer URL is missing application`,
}, {
	s:   "/user/model",
	err: `application offer URL is missing application`,
}, {
	s:   "model.application@bad",
	err: `application name "application@bad" not valid`,
}, {
	s:   "user[bad/model.application",
	err: `user name "user\[bad" not valid`,
}, {
	s:   "user/[badmodel.application",
	err: `model name "\[badmodel" not valid`,
}}

func (s *OfferURLSuite) TestParseURL(c *gc.C) {
	for i, t := range urlTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseOfferURL(t.s)

		match := t.s
		if t.exact != "" {
			match = t.exact
		}
		if t.url != nil {
			c.Assert(err, gc.IsNil)
			c.Check(url, gc.DeepEquals, t.url)
			c.Check(url.String(), gc.Equals, match)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Check(err, gc.ErrorMatches, t.err)
			c.Check(url, gc.IsNil)
		}
	}
}

var urlPartsTests = []struct {
	s, err string
	url    *crossmodel.OfferURLParts
}{{
	s:   "controller:/user/modelname.applicationname",
	url: &crossmodel.OfferURLParts{"controller", "user", "modelname", "applicationname"},
}, {
	s:   "user/modelname.applicationname",
	url: &crossmodel.OfferURLParts{"", "user", "modelname", "applicationname"},
}, {
	s:   "user/modelname",
	url: &crossmodel.OfferURLParts{"", "user", "modelname", ""},
}, {
	s:   "modelname.application",
	url: &crossmodel.OfferURLParts{"", "", "modelname", "application"},
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
	s:   "user/prod/applicationname/extra",
	err: `application offer URL has invalid form, must be \[<user/\]<model>.<appname>: "user/prod/applicationname/extra"`,
}, {
	s:   "controller:/user/modelname.application@bad",
	err: `application name "application@bad" not valid`,
}, {
	s:   "controller:/user[bad/modelname.application",
	err: `user name "user\[bad" not valid`,
}, {
	s:   ":foo",
	err: `application offer URL has invalid form, must be \[<user/\]<model>.<appname>: $URL`,
}}

func (s *OfferURLSuite) TestParseURLParts(c *gc.C) {
	for i, t := range urlPartsTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseOfferURLParts(t.s)

		if t.url != nil {
			c.Check(err, gc.IsNil)
			c.Check(url, gc.DeepEquals, t.url)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Assert(err, gc.ErrorMatches, t.err)
			c.Assert(url, gc.IsNil)
		}
	}
}

func (s *OfferURLSuite) TestHasEndpoint(c *gc.C) {
	url, err := crossmodel.ParseOfferURL("model.application:endpoint")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsTrue)
	url, err = crossmodel.ParseOfferURL("model.application")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsFalse)
	url, err = crossmodel.ParseOfferURL("controller:/user/model.application:thing")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsTrue)
	url, err = crossmodel.ParseOfferURL("controller:/user/model.application")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsFalse)
}

func (s *OfferURLSuite) TestMakeURL(c *gc.C) {
	url := crossmodel.MakeURL("user", "model", "app", "")
	c.Assert(url, gc.Equals, "user/model.app")
	url = crossmodel.MakeURL("user", "model", "app", "ctrl")
	c.Assert(url, gc.Equals, "ctrl:user/model.app")
}

func (s *OfferURLSuite) TestAsLocal(c *gc.C) {
	url, err := crossmodel.ParseOfferURL("source:model.application:endpoint")
	c.Assert(err, jc.ErrorIsNil)
	expected, err := crossmodel.ParseOfferURL("model.application:endpoint")
	c.Assert(err, jc.ErrorIsNil)
	original := *url
	c.Assert(url.AsLocal(), gc.DeepEquals, expected)
	c.Assert(*url, gc.DeepEquals, original)
}
