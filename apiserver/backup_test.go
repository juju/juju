// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apiserverbackups "github.com/juju/juju/apiserver/facades/client/backups"
	"github.com/juju/juju/apiserver/params"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type backupsCommonSuite struct {
	apiserverBaseSuite
	backupURL string
	fake      *backupstesting.FakeBackups
}

func (s *backupsCommonSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)

	s.backupURL = s.server.URL + fmt.Sprintf("/model/%s/backups", s.State.ModelUUID())
	s.fake = &backupstesting.FakeBackups{}
	s.PatchValue(apiserver.NewBackups,
		func(st *state.State, m *state.Model) (backups.Backups, io.Closer) {
			return s.fake, ioutil.NopCloser(nil)
		},
	)
}

func (s *backupsCommonSuite) assertErrorResponse(c *gc.C, resp *http.Response, statusCode int, msg string) *params.Error {
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(resp.StatusCode, gc.Equals, statusCode, gc.Commentf("body: %s", body))
	c.Assert(resp.Header.Get("Content-Type"), gc.Equals, params.ContentTypeJSON, gc.Commentf("body: %q", body))

	var failure params.Error
	err = json.Unmarshal(body, &failure)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&failure, gc.ErrorMatches, msg, gc.Commentf("body: %s", body))
	return &failure
}

type backupsSuite struct {
	backupsCommonSuite
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) TestRequiresAuth(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: s.backupURL})
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *backupsSuite) checkInvalidMethod(c *gc.C, method, url string) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: method, URL: url})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "`+method+`"`)
}

func (s *backupsSuite) TestInvalidHTTPMethods(c *gc.C) {
	url := s.backupURL
	for _, method := range []string{"POST", "DELETE", "OPTIONS"} {
		c.Log("testing HTTP method: " + method)
		s.checkInvalidMethod(c, method, url)
	}
}

func (s *backupsSuite) TestAuthRequiresClientNotMachine(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Tag:      machine.Tag().String(),
		Password: password,
		Method:   "GET",
		URL:      s.backupURL,
		Nonce:    "fake_nonce",
	})
	s.assertErrorResponse(c, resp, http.StatusInternalServerError, "tag kind machine not valid")

	// Now try a user login.
	resp = s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "POST", URL: s.backupURL})
	s.assertErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "POST"`)
}

type backupsDownloadSuite struct {
	backupsCommonSuite
}

var _ = gc.Suite(&backupsDownloadSuite{})

// sendValid sends a valid GET request to the backups endpoint
// and returns the response and the expected contents of the
// archive if the request succeeds.
func (s *backupsDownloadSuite) sendValidGet(c *gc.C) (resp *http.Response, archiveBytes []byte) {
	meta := backupstesting.NewMetadata()
	archive, err := backupstesting.NewArchiveBasic(meta)
	c.Assert(err, jc.ErrorIsNil)
	archiveBytes = archive.Bytes()
	s.fake.Meta = meta
	s.fake.Archive = ioutil.NopCloser(archive)

	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "GET",
		URL:         s.backupURL,
		ContentType: params.ContentTypeJSON,
		JSONBody: params.BackupsDownloadArgs{
			ID: meta.ID(),
		},
	}), archiveBytes
}

func (s *backupsDownloadSuite) TestCalls(c *gc.C) {
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()

	c.Check(s.fake.Calls, gc.DeepEquals, []string{"Get"})
	c.Check(s.fake.IDArg, gc.Equals, s.fake.Meta.ID())
}

func (s *backupsDownloadSuite) TestResponse(c *gc.C) {
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()
	meta := s.fake.Meta

	c.Check(resp.StatusCode, gc.Equals, http.StatusOK)
	expectedChecksum := base64.StdEncoding.EncodeToString([]byte(meta.Checksum()))
	c.Check(resp.Header.Get("Digest"), gc.Equals, string(params.DigestSHA256)+"="+expectedChecksum)
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, params.ContentTypeRaw)
}

func (s *backupsDownloadSuite) TestBody(c *gc.C) {
	resp, archiveBytes := s.sendValidGet(c)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(body, jc.DeepEquals, archiveBytes)
}

func (s *backupsDownloadSuite) TestErrorWhenGetFails(c *gc.C) {
	s.fake.Error = errors.New("failed!")
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()

	s.assertErrorResponse(c, resp, http.StatusInternalServerError, "failed!")
}

type backupsUploadSuite struct {
	backupsCommonSuite
	meta *backups.Metadata
}

var _ = gc.Suite(&backupsUploadSuite{})

func (s *backupsUploadSuite) sendValid(c *gc.C, id string) *http.Response {
	s.fake.Meta = backups.NewMetadata()
	s.fake.Meta.SetID("<a new backup ID>")

	var parts bytes.Buffer
	writer := multipart.NewWriter(&parts)

	// Set the metadata part.
	s.meta = backups.NewMetadata()
	metaResult := apiserverbackups.CreateResult(s.meta, "test-filename")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="metadata"`)
	header.Set("Content-Type", params.ContentTypeJSON)
	part, err := writer.CreatePart(header)
	c.Assert(err, jc.ErrorIsNil)
	err = json.NewEncoder(part).Encode(metaResult)
	c.Assert(err, jc.ErrorIsNil)

	// Set the attached part.
	archive := bytes.NewBufferString("<compressed data>")
	part, err = writer.CreateFormFile("attached", "juju-backup.tar.gz")
	c.Assert(err, jc.ErrorIsNil)
	_, err = io.Copy(part, archive)
	c.Assert(err, jc.ErrorIsNil)

	// Send the request.
	ctype := writer.FormDataContentType()
	return s.sendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "PUT", URL: s.backupURL, ContentType: ctype, Body: &parts})
}

func (s *backupsUploadSuite) TestCalls(c *gc.C) {
	resp := s.sendValid(c, "<a new backup ID>")
	defer resp.Body.Close()

	c.Check(s.fake.Calls, gc.DeepEquals, []string{"Add"})
	c.Check(s.fake.ArchiveArg, gc.NotNil)
	c.Check(s.fake.MetaArg, jc.DeepEquals, s.meta)
}

func (s *backupsUploadSuite) TestResponse(c *gc.C) {
	resp := s.sendValid(c, "<a new backup ID>")
	defer resp.Body.Close()

	c.Check(resp.StatusCode, gc.Equals, http.StatusOK)
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, params.ContentTypeJSON)
}

func (s *backupsUploadSuite) TestBody(c *gc.C) {
	resp := s.sendValid(c, "<a new backup ID>")
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	var result params.BackupsUploadResult
	err = json.Unmarshal(body, &result)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.ID, gc.Equals, "<a new backup ID>")
}

func (s *backupsUploadSuite) TestErrorWhenGetFails(c *gc.C) {
	s.fake.Error = errors.New("failed!")
	resp := s.sendValid(c, "<a new backup ID>")
	defer resp.Body.Close()

	s.assertErrorResponse(c, resp, http.StatusInternalServerError, "failed!")
}
