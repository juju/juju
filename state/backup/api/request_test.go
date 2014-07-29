// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"encoding/base64"
	"net/url"

	gc "launchpad.net/gocheck"

	backup "github.com/juju/juju/state/backup/api"
)

//---------------------------
// NewAPIRequest()

func (b *BackupSuite) TestNewAPIRequest(c *gc.C) {
	URL, err := url.Parse("https://localhost:8080")
	c.Assert(err, gc.IsNil)
	uuid := "abc-xyz"
	tag := "someuser"
	pw := "password"
	req, err := backup.NewAPIRequest(URL, uuid, tag, pw)
	c.Check(err, gc.IsNil)

	c.Check(req.Method, gc.Equals, "POST")
	c.Check(req.URL.String(), gc.Matches, `https://localhost:8080/environment/[-\w]+/backup`)
	c.Check(req.Body, gc.IsNil)

	auth := req.Header.Get("Authorization")
	cred := base64.StdEncoding.EncodeToString([]byte(tag + ":" + pw))
	c.Check(auth, gc.Equals, "Basic "+cred)
}
