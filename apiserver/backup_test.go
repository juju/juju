// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/apiserver"
	apitesting "github.com/juju/juju/v2/apiserver/testing"
	"github.com/juju/juju/v2/rpc/params"
	"github.com/juju/juju/v2/state"
	"github.com/juju/juju/v2/state/backups"
	backupstesting "github.com/juju/juju/v2/state/backups/testing"
)

var _ = gc.Suite(&backupsSuite{})

type backupsSuite struct {
	apiserverBaseSuite
	backupURL string
	fake      *backupstesting.FakeBackups
}

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)

	s.backupURL = s.server.URL + fmt.Sprintf("/model/%s/backups", s.State.ModelUUID())
	s.fake = &backupstesting.FakeBackups{}
	s.PatchValue(apiserver.NewBackups,
		func() backups.Backups {
			return s.fake
		},
	)
}

func (s *backupsSuite) assertErrorResponse(c *gc.C, resp *http.Response, statusCode int, msg string) *params.Error {
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
	for _, method := range []string{"PUT", "POST", "DELETE", "OPTIONS"} {
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

// sendValid sends a valid GET request to the backups endpoint
// and returns the response and the expected contents of the
// archive if the request succeeds.
func (s *backupsSuite) sendValidGet(c *gc.C) (resp *http.Response, archiveBytes []byte) {
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

func (s *backupsSuite) TestCalls(c *gc.C) {
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()

	c.Check(s.fake.Calls, gc.DeepEquals, []string{"Get"})
	c.Check(s.fake.IDArg, gc.Equals, s.fake.Meta.ID())
}

func (s *backupsSuite) TestResponse(c *gc.C) {
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()
	meta := s.fake.Meta

	c.Check(resp.StatusCode, gc.Equals, http.StatusOK)
	expectedChecksum := base64.StdEncoding.EncodeToString([]byte(meta.Checksum()))
	c.Check(resp.Header.Get("Digest"), gc.Equals, string(params.DigestSHA256)+"="+expectedChecksum)
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, params.ContentTypeRaw)
}

func (s *backupsSuite) TestBody(c *gc.C) {
	resp, archiveBytes := s.sendValidGet(c)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(body, jc.DeepEquals, archiveBytes)
}

func (s *backupsSuite) TestErrorWhenGetFails(c *gc.C) {
	s.fake.Error = errors.New("failed!")
	resp, _ := s.sendValidGet(c)
	defer resp.Body.Close()

	s.assertErrorResponse(c, resp, http.StatusInternalServerError, "failed!")
}
