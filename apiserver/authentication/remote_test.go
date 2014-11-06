// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"encoding/base64"
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

var _ state.Entity = (*authentication.RemoteUser)(nil)

var _ authentication.EntityAuthenticator = (*authentication.RemoteAuthenticator)(nil)

type RemoteAuthSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&RemoteAuthSuite{})

func (*RemoteAuthSuite) TestRemoteCredentialsRoundTrip(c *gc.C) {
	credentials := `eyJQcmltYXJ5Ijp7ImNhdmVhdHMiOlt7ImNpZCI6ImV5SlVhR2x5WkZCaGNuUjVVSFZpYkdsalMyVjVJam9pZFROUmNYVnZSbGR1ZVRKSWRtZE9OVVl5VFZCbFN6Qk5Oa0ZDTUVRMFVYTTNlVWhaYWs5WVZTdDVZejBpTENKR2FYSnpkRkJoY25SNVVIVmliR2xqUzJWNUlqb2lSa0ZHVUZOSmFVSlJReXRqV1ZKdk5FOHZWVEUyY0ZoSlUwRndPRFJsYWpGM2RFNHJXR1JtVjFWQlRUMGlMQ0pPYjI1alpTSTZJa1JTVkVWdmQyNXhOM1pyWVZScFNXWkdLMXByVFhCUmNsWmplamhrYUhaVElpd2lTV1FpT2lKc1FYSm1WR2MxYzFWRk5ESjBkVE0zU1hsWlN6WlBUVUk1ZEZvME5URmFTbmN6TTNSS1dtRm5kRUpQY0dWeFYxRnpkVlIwUVVKb1prRkpjbm95VUN0S2IyTmxhRE0xWVVOaWNFZGtNalZWVTJsS2MyaEpUMDVuZWtkTk0ycG1NMDg0WTBSbGVsWmxlRUo2UlZZNVVGUkhjVUZSV0hJNFowOWpRa0pYVERGbGVHcHNUakk1YUdka0sxVTNjU0o5IiwidmlkIjoiWmtXTUN0M1JUaG41cGtHa0dEU1ZFSlBwaHZ2aDhmbG82djZ4eUtHMndUSjVwN0tpSVA0a2U3L0R6OXlGRFZyNWtMOFlxRlc5TlV6T2M0ZTN3VUZSc2c9PSIsImNsIjoicmVtb3RlLXNlcnZpY2UtbG9jYXRpb24ifSx7ImNpZCI6ImlzLWJlZm9yZS10aW1lPyAyMDE0LTExLTE5VDIzOjIyOjI3WiJ9XSwibG9jYXRpb24iOiI5MDE2OGU0Yy0yZjEwLTRlOWMtODNjMi1mZWVkZmFjZWU1YTkiLCJpZGVudGlmaWVyIjoiNjBjY2QzYzcxYWQ3OWM2ZTE2ODAzZWE0NWQ3ZDkzN2YxNmJhMTUxMDRhYTQ1MzFlIiwic2lnbmF0dXJlIjoiNTFmOGE1MTk0N2RjYWFkYjk4ZDcwZWQxYjVlYTMzMzM0OGUzZDEwNDJlODc1ZDJmMjkzMGM4OTZkNjYwNGVlYSJ9LCJEaXNjaGFyZ2VzIjpudWxsfQ==`
	var remoteCreds authentication.RemoteCredentials
	err := remoteCreds.UnmarshalText([]byte(credentials))
	c.Assert(err, gc.IsNil)
	c.Assert(remoteCreds.Primary, gc.NotNil)
	c.Assert(remoteCreds.Discharges, gc.HasLen, 0)
	c.Assert(remoteCreds.Primary.Id(), gc.Equals, "60ccd3c71ad79c6e16803ea45d7d937f16ba15104aa4531e")
	out, err := remoteCreds.MarshalText()
	c.Assert(err, gc.IsNil)
	c.Assert(string(out), gc.Equals, credentials)
}

func (*RemoteAuthSuite) TestMalformedRemoteCredentials(c *gc.C) {
	var remoteCreds authentication.RemoteCredentials

	err := remoteCreds.UnmarshalText([]byte("|||invalid base64|||"))
	c.Assert(err, gc.NotNil)
	c.Assert(authentication.IsMalformedRemoteCredentialsErr(err), jc.IsTrue)

	err = remoteCreds.UnmarshalText([]byte(
		base64.URLEncoding.EncodeToString([]byte("}}}invalid json{{{"))))
	c.Assert(err, gc.NotNil)
	c.Assert(authentication.IsMalformedRemoteCredentialsErr(err), jc.IsTrue)

	err = remoteCreds.UnmarshalText([]byte(
		base64.URLEncoding.EncodeToString([]byte("{}"))))
	c.Assert(err, gc.ErrorMatches, "missing primary credential")
}

func (*RemoteAuthSuite) TestIsBeforeTime(c *gc.C) {
	var ra *authentication.RemoteAuthenticator
	now := time.Now().UTC()
	testCases := []struct {
		predicate  string
		expiration time.Time
		override   string
		pattern    string
	}{
		{
			expiration: now.Add(time.Hour),
			pattern:    "",
		},
		{
			override: "3am eternal",
			pattern:  ".* invalid expiration .*",
		},
		{
			expiration: now.Add(0 - time.Hour),
			pattern:    ".* authorization expired at .*",
		},
	}
	for _, testCase := range testCases {
		var caveat string
		if !testCase.expiration.IsZero() {
			caveat = fmt.Sprintf("is-before-time? %s", testCase.expiration.Format(time.RFC3339))
		} else if testCase.override != "" {
			caveat = fmt.Sprintf("is-before-time? %s", testCase.override)
		} else {
			c.Fail()
		}
		err := ra.CheckFirstPartyCaveat(caveat)
		if testCase.pattern == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, testCase.pattern)
		}
	}
}
