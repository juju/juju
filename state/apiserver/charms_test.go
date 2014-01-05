// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
)

type charmsSuite struct {
	jujutesting.JujuConnSuite
	userTag  string
	password string
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	user, err := s.State.AddUser("joe", password)
	c.Assert(err, gc.IsNil)
	s.userTag = user.Tag()
	s.password = password
}

func (s *charmsSuite) TestCharmsServedSecurely(c *gc.C) {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	uri := "http://" + info.Addrs[0] + "/charms"
	_, err = s.sendRequest(c, "", "", "GET", uri, "", nil)
	c.Assert(err, gc.ErrorMatches, `.*malformed HTTP response.*`)
}

func (s *charmsSuite) TestRequiresAuth(c *gc.C) {
	resp, err := s.sendRequest(c, "", "", "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusUnauthorized, "unauthorized", "")
}

func (s *charmsSuite) TestUploadRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`, "")
}

func (s *charmsSuite) TestAuthRequiresUser(c *gc.C) {
	// Add a machine and try to login.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", "fake_nonce", nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)

	resp, err := s.sendRequest(c, machine.Tag(), password, "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusUnauthorized, "unauthorized", "")

	// Now try a user login.
	resp, err = s.authRequest(c, "GET", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`, "")
}

func (s *charmsSuite) TestUploadRequiresSeries(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.charmsURI(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "expected series= URL argument", "")
}

func (s *charmsSuite) TestUploadFailsWithInvalidZip(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "invalid charm archive: zip: not a valid zip file", "")

	// Now try with the default Content-Type.
	resp, err = s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), false, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "expected Content-Type: application/zip, got: application/octet-stream", "")
}

func (s *charmsSuite) TestUploadBumpsRevision(c *gc.C) {
	// Add the dummy charm with revision 1.
	ch := coretesting.Charms.Bundle(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.testing.invalid/dummy-1")
	c.Assert(err, gc.IsNil)
	_, err = s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, gc.IsNil)

	// Now try uploading the same revision and verify it gets bumped,
	// and the BundleURL and BundleSha256 are calculated.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-2")
	s.assertResponse(c, resp, http.StatusOK, "", expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 2)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
	// No more checks for these two here, because they
	// are verified in TestUploadRequiresSingleUploadedFile.
	c.Assert(sch.BundleURL(), gc.Not(gc.Equals), "")
	c.Assert(sch.BundleSha256(), gc.Not(gc.Equals), "")
}

func (s *charmsSuite) TestUploadRespectsLocalRevision(c *gc.C) {
	// Make a dummy charm dir with revision 123.
	dir := coretesting.Charms.ClonedDir(c.MkDir(), "dummy")
	dir.SetDiskRevision(123)
	// Now bundle the dir.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = dir.BundleTo(tempFile)
	c.Assert(err, gc.IsNil)

	// Now try uploading it and ensure the revision persists.
	resp, err := s.uploadRequest(c, s.charmsURI(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	expectedURL := charm.MustParseURL("local:quantal/dummy-123")
	s.assertResponse(c, resp, http.StatusOK, "", expectedURL.String())
	sch, err := s.State.Charm(expectedURL)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedURL)
	c.Assert(sch.Revision(), gc.Equals, 123)
	c.Assert(sch.IsUploaded(), jc.IsTrue)

	// First rewind the reader, which was reset but BundleTo() above.
	_, err = tempFile.Seek(0, 0)
	c.Assert(err, gc.IsNil)

	// Finally, verify the SHA256 and uploaded URL.
	expectedSHA256, _, err := getSHA256(tempFile)
	c.Assert(err, gc.IsNil)
	name := charm.Quote(expectedURL.String())
	storage, err := apiserver.GetEnvironStorage(s.State)
	c.Assert(err, gc.IsNil)
	expectedUploadURL, err := storage.URL(name)
	c.Assert(err, gc.IsNil)

	c.Assert(sch.BundleURL().String(), gc.Equals, expectedUploadURL)
	c.Assert(sch.BundleSha256(), gc.Equals, expectedSHA256)
}

func (s *charmsSuite) charmsURI(c *gc.C, query string) string {
	_, info, err := s.APIConn.Environ.StateInfo()
	c.Assert(err, gc.IsNil)
	return "https://" + info.Addrs[0] + "/charms" + query
}

func (s *charmsSuite) sendRequest(c *gc.C, tag, password, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, uri, body)
	c.Assert(err, gc.IsNil)
	if tag != "" && password != "" {
		req.SetBasicAuth(tag, password)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return utils.GetNonValidatingHTTPClient().Do(req)
}

func (s *charmsSuite) authRequest(c *gc.C, method, uri, contentType string, body io.Reader) (*http.Response, error) {
	return s.sendRequest(c, s.userTag, s.password, method, uri, contentType, body)
}

func (s *charmsSuite) uploadRequest(c *gc.C, uri string, asZip bool, path string) (*http.Response, error) {
	contentType := "application/octet-stream"
	if asZip {
		contentType = "application/zip"
	}

	if path == "" {
		return s.authRequest(c, "POST", uri, contentType, nil)
	}

	file, err := os.Open(path)
	c.Assert(err, gc.IsNil)
	defer file.Close()
	return s.authRequest(c, "POST", uri, contentType, file)
}

func (s *charmsSuite) assertResponse(c *gc.C, resp *http.Response, expCode int, expError, expCharmURL string) {
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	c.Assert(err, gc.IsNil)
	var jsonResponse params.CharmsResponse
	err = json.Unmarshal(body, &jsonResponse)
	c.Assert(err, gc.IsNil)
	if expError != "" {
		c.Check(jsonResponse.Error, gc.Matches, expError)
		c.Check(jsonResponse.CharmURL, gc.Equals, "")
	} else {
		c.Check(jsonResponse.Error, gc.Equals, "")
		c.Check(jsonResponse.CharmURL, gc.Equals, expCharmURL)
	}
	c.Check(resp.StatusCode, gc.Equals, expCode)
}

func getSHA256(source io.ReadSeeker) (string, int64, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, source)
	if err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	return digest, size, nil
}
