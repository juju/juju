// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type baseBackupsSuite struct {
	authHttpSuite
	fake *backupstesting.FakeBackups
}

func (s *baseBackupsSuite) SetUpTest(c *gc.C) {
	s.authHttpSuite.SetUpTest(c)

	s.fake = &backupstesting.FakeBackups{}
	s.PatchValue(apiserver.NewBackups,
		func(st *state.State) (backups.Backups, error) {
			return s.fake, nil
		},
	)
}

func (s *baseBackupsSuite) backupURL(c *gc.C) string {
	environ, err := s.State.Environment()
	c.Assert(err, gc.IsNil)
	uri := s.baseURL(c)
	uri.Path = fmt.Sprintf("/environment/%s/backup", environ.UUID())
	return uri.String()
}

func (s *baseBackupsSuite) checkErrorResponse(c *gc.C, resp *http.Response, statusCode int, msg string) {
	c.Check(resp.StatusCode, gc.Equals, statusCode)
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, apihttp.CTYPE_JSON)

	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)

	var failure params.Error
	err = json.Unmarshal(body, &failure)
	c.Assert(err, gc.IsNil)
	c.Check(&failure, gc.ErrorMatches, msg)
}

type backupsSuite struct {
	baseBackupsSuite
}

var _ = gc.Suite(&backupsSuite{})

func (s *backupsSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")
}

func (s *backupsSuite) checkInvalidMethod(c *gc.C, method, url string) {
	resp, err := s.authRequest(c, method, url, "", nil)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "`+method+`"`)
}

func (s *backupsSuite) TestRequiresGET(c *gc.C) {
	url := s.backupURL(c)
	for _, method := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
		s.checkInvalidMethod(c, method, url)
	}
}

func (s *backupsSuite) TestAuthRequiresClientNotMachine(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)

	resp, err := s.sendRequest(c, machine.Tag().String(), password, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusUnauthorized, "unauthorized")

	// Now try a user login.
	resp, err = s.authRequest(c, "POST", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	s.checkErrorResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "POST"`)
}

func (s *backupsSuite) TestErrorWhenNewBackupsFails(c *gc.C) {
	s.PatchValue(apiserver.NewBackups,
		func(st *state.State) (backups.Backups, error) {
			return nil, errors.New("failed!")
		},
	)

	resp, err := s.authRequest(c, "GET", s.backupURL(c), "", nil)
	c.Assert(err, gc.IsNil)
	defer resp.Body.Close()

	s.checkErrorResponse(c, resp, 500, "failed!")
}

type backupsDownloadSuite struct {
	baseBackupsSuite
	body []byte
}

var _ = gc.Suite(&backupsDownloadSuite{})

func (s *backupsDownloadSuite) newBody(c *gc.C, id string) *bytes.Buffer {
	args := params.BackupsDownloadArgs{
		ID: id,
	}
	body, err := json.Marshal(args)
	c.Assert(err, gc.IsNil)
	return bytes.NewBuffer(body)
}

func (s *backupsDownloadSuite) sendValid(c *gc.C) *http.Response {
	meta := backupstesting.NewMetadata()
	archive, err := backupstesting.NewArchiveBasic(meta)
	c.Assert(err, gc.IsNil)
	s.fake.Meta = meta
	s.fake.Archive = ioutil.NopCloser(archive)
	s.body = archive.Bytes()

	ctype := apihttp.CTYPE_JSON
	body := s.newBody(c, meta.ID())
	resp, err := s.authRequest(c, "GET", s.backupURL(c), ctype, body)
	c.Assert(err, gc.IsNil)
	return resp
}

func (s *backupsDownloadSuite) TestCalls(c *gc.C) {
	resp := s.sendValid(c)
	defer resp.Body.Close()

	c.Check(s.fake.Calls, gc.DeepEquals, []string{"Get"})
	c.Check(s.fake.IDArg, gc.Equals, s.fake.Meta.ID())
}

func (s *backupsDownloadSuite) TestResponse(c *gc.C) {
	resp := s.sendValid(c)
	defer resp.Body.Close()
	meta := s.fake.Meta

	c.Check(resp.StatusCode, gc.Equals, 200)
	c.Check(resp.Header.Get("Digest"), gc.Equals, string(apihttp.DIGEST_SHA)+"="+meta.Checksum())
	c.Check(resp.Header.Get("Content-Type"), gc.Equals, apihttp.CTYPE_RAW)
}

func (s *backupsDownloadSuite) TestBody(c *gc.C) {
	resp := s.sendValid(c)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, gc.IsNil)
	c.Check(body, jc.DeepEquals, s.body)
}

func (s *backupsDownloadSuite) TestErrorWhenGetFails(c *gc.C) {
	s.fake.Error = errors.New("failed!")
	resp := s.sendValid(c)
	defer resp.Body.Close()

	s.checkErrorResponse(c, resp, 500, "failed!")
}
