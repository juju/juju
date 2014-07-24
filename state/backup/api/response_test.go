// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/api/params"
	backup "github.com/juju/juju/state/backup/api"
	"github.com/juju/juju/testing"
)

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
	resp := http.Response{
		Body: &testing.FakeFile{ReadError: "failed to read"},
	}
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
		Body:       &testing.FakeFile{ReadError: "failed to read"},
		StatusCode: http.StatusInternalServerError,
	}
	err := backup.CheckAPIResponse(&resp)

	c.Check(err, gc.ErrorMatches, `\(could not read HTTP response: failed to read\)`)
}

//---------------------------
// ExtractFilename()

func (b *BackupSuite) TestExtractFilename(c *gc.C) {
	header := http.Header{}
	header.Set("Content-Disposition", `attachment; filename="backup.tar.gz"`)
	filename, err := backup.ExtractFilename(header)

	c.Check(err, gc.IsNil)
	c.Check(filename, gc.Equals, "backup.tar.gz")
}

func (b *BackupSuite) TestExtractFilenameHeaderMissing(c *gc.C) {
	header := http.Header{}
	_, err := backup.ExtractFilename(header)

	c.Check(err, gc.ErrorMatches, "no valid header found")
}

func (b *BackupSuite) TestExtractFilenameHeaderEmpty(c *gc.C) {
	header := http.Header{}
	header.Set("Content-Disposition", "")
	_, err := backup.ExtractFilename(header)

	c.Check(err, gc.ErrorMatches, "no valid header found")
}

func (b *BackupSuite) TestExtractFilenameMalformed(c *gc.C) {
	header := http.Header{}
	header.Set("Content-Disposition", "something unexpected")
	_, err := backup.ExtractFilename(header)

	c.Check(err, gc.ErrorMatches, "no valid header found")
}
