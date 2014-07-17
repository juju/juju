// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"encoding/base64"
	"net/http"
	"net/url"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/backup"
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
	c.Check(req.URL.String(), gc.Equals, "https://localhost:8080/backup")
	c.Check(req.Body, gc.IsNil)

	auth := req.Header.Get("Authorization")
	cred := base64.StdEncoding.EncodeToString([]byte(tag + ":" + pw))
	c.Check(auth, gc.Equals, "Basic "+cred)
}

//---------------------------
// parseJSONError()

func (b *BackupSuite) TestParseJSONError(c *gc.C) {
	msg := "failed!"
	code := ""
	resp := b.newHTTPFailure(c, msg, code)
	failure, err := backup.ParseJSONError(resp)
	c.Check(err, gc.IsNil)
	c.Check(failure, gc.Equals, "failed!")
}

func (b *BackupSuite) TestParseJSONErrorBadBody(c *gc.C) {
	resp := http.Response{Body: &badReadWriter{}}
	_, err := backup.ParseJSONError(&resp)
	c.Check(err, gc.ErrorMatches, "could not read HTTP response: failed to read")
}

func (b *BackupSuite) TestParseJSONErrorBadJSON(c *gc.C) {
	resp := b.newDataResponse(c, "not valid json")
	_, err := backup.ParseJSONError(resp)
	c.Check(err, gc.ErrorMatches, "could not extract error from HTTP response: .*")
}

func (b *BackupSuite) TestParseJSONErrorEmptyBody(c *gc.C) {
	resp := b.newDataResponse(c, "")
	_, err := backup.ParseJSONError(resp)
	c.Check(err, gc.ErrorMatches, "could not extract error from HTTP response: .*")
}

func (b *BackupSuite) TestParseJSONErrorWrongType(c *gc.C) {
	badfailure := struct{ Spam int }{5}
	resp := b.newJSONResponse(c, 500, &badfailure)
	failure, err := backup.ParseJSONError(resp)
	c.Check(err, gc.IsNil)
	c.Check(failure, gc.Equals, "")
}

//---------------------------
// CheckAPIResponse()

func (b *BackupSuite) TestCheckAPIResponse(c *gc.C) {
	resp := b.newDataResponse(c, "")
	err := backup.CheckAPIResponse(resp)
	c.Check(err, gc.IsNil)
}

func (b *BackupSuite) TestCheckAPIResponseISE(c *gc.C) {
	resp := b.newHTTPFailure(c, "failed!", "")
	err := backup.CheckAPIResponse(resp)

	c.Check(params.ErrCode(err), gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "failed!")
}

func (b *BackupSuite) TestCheckAPIResponseStatusMismatch(c *gc.C) {
	resp := b.newHTTPFailure(c, "failed!", "")
	resp.StatusCode = http.StatusOK
	err := backup.CheckAPIResponse(resp)
	c.Check(err, gc.IsNil)
}

func (b *BackupSuite) TestCheckAPIResponseStatusNotFound(c *gc.C) {
	resp := b.newHTTPFailure(c, "failed!", "")
	resp.StatusCode = http.StatusNotFound
	err := backup.CheckAPIResponse(resp)

	c.Check(err, jc.Satisfies, params.IsCodeNotImplemented)
	c.Check(err, gc.ErrorMatches, "failed!")
}

func (b *BackupSuite) TestCheckAPIResponseStatusMethodNotAllowed(c *gc.C) {
	resp := b.newHTTPFailure(c, "failed!", "")
	resp.StatusCode = http.StatusMethodNotAllowed
	err := backup.CheckAPIResponse(resp)

	c.Check(err, jc.Satisfies, params.IsCodeNotImplemented)
	c.Check(err, gc.ErrorMatches, "failed!")
}

func (b *BackupSuite) TestCheckAPIResponseStatusUnauthorized(c *gc.C) {
	resp := b.newHTTPFailure(c, "failed!", "")
	resp.StatusCode = http.StatusUnauthorized
	err := backup.CheckAPIResponse(resp)

	c.Check(params.ErrCode(err), gc.Equals, "")
	c.Check(err, gc.ErrorMatches, "failed!")
}

func (b *BackupSuite) TestCheckAPIResponseBadBody(c *gc.C) {
	resp := http.Response{
		Body:       &badReadWriter{},
		StatusCode: http.StatusInternalServerError,
	}
	err := backup.CheckAPIResponse(&resp)

	c.Check(err, gc.ErrorMatches, `\(could not read HTTP response: failed to read\)`)
}
