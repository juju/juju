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

type ApplicationURLSuite struct{}

var _ = gc.Suite(&ApplicationURLSuite{})

var urlTests = []struct {
	s, err string
	exact  string
	url    *crossmodel.ApplicationURL
}{{
	s:   "controller:user/modelname.applicationname",
	url: &crossmodel.ApplicationURL{"controller", "user", "modelname", "applicationname"},
}, {
	s:   "controller:user/modelname.applicationname:rel",
	url: &crossmodel.ApplicationURL{"controller", "user", "modelname", "applicationname:rel"},
}, {
	s:   "modelname.applicationname",
	url: &crossmodel.ApplicationURL{"", "", "modelname", "applicationname"},
}, {
	s:   "modelname.applicationname:rel",
	url: &crossmodel.ApplicationURL{"", "", "modelname", "applicationname:rel"},
}, {
	s:   "user/modelname.applicationname:rel",
	url: &crossmodel.ApplicationURL{"", "user", "modelname", "applicationname:rel"},
}, {
	s:     "/modelname.applicationname",
	url:   &crossmodel.ApplicationURL{"", "", "modelname", "applicationname"},
	exact: "modelname.applicationname",
}, {
	s:     "/modelname.applicationname:rel",
	url:   &crossmodel.ApplicationURL{"", "", "modelname", "applicationname:rel"},
	exact: "modelname.applicationname:rel",
}, {
	s:   "user/modelname.applicationname",
	url: &crossmodel.ApplicationURL{"", "user", "modelname", "applicationname"},
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

func (s *ApplicationURLSuite) TestParseURL(c *gc.C) {
	for i, t := range urlTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseApplicationURL(t.s)

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
	url    *crossmodel.ApplicationURLParts
}{{
	s:   "controller:/user/modelname.applicationname",
	url: &crossmodel.ApplicationURLParts{"controller", "user", "modelname", "applicationname"},
}, {
	s:   "user/modelname.applicationname",
	url: &crossmodel.ApplicationURLParts{"", "user", "modelname", "applicationname"},
}, {
	s:   "user/modelname",
	url: &crossmodel.ApplicationURLParts{"", "user", "modelname", ""},
}, {
	s:   "modelname.application",
	url: &crossmodel.ApplicationURLParts{"", "", "modelname", "application"},
}, {
	s:   "controller:/modelname",
	url: &crossmodel.ApplicationURLParts{"controller", "", "modelname", ""},
}, {
	s:   "controller:",
	url: &crossmodel.ApplicationURLParts{Source: "controller"},
}, {
	s:   "",
	url: &crossmodel.ApplicationURLParts{},
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

func (s *ApplicationURLSuite) TestParseURLParts(c *gc.C) {
	for i, t := range urlPartsTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseApplicationURLParts(t.s)

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

func (s *ApplicationURLSuite) TestHasEndpoint(c *gc.C) {
	url, err := crossmodel.ParseApplicationURL("model.application:endpoint")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsTrue)
	url, err = crossmodel.ParseApplicationURL("model.application")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsFalse)
	url, err = crossmodel.ParseApplicationURL("controller:/user/model.application:thing")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsTrue)
	url, err = crossmodel.ParseApplicationURL("controller:/user/model.application")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.HasEndpoint(), jc.IsFalse)
}

func (s *ApplicationURLSuite) TestMakeURL(c *gc.C) {
	url := crossmodel.MakeURL("user", "model", "app", "")
	c.Assert(url, gc.Equals, "user/model.app")
	url = crossmodel.MakeURL("user", "model", "app", "ctrl")
	c.Assert(url, gc.Equals, "ctrl:user/model.app")
}
