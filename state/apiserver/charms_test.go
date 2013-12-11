// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
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
	resp, err := s.sendRequest(c, "", "", "GET", s.charmsUri(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusUnauthorized, "unauthorized", "")
}

func (s *charmsSuite) TestUploadRequiresPOST(c *gc.C) {
	resp, err := s.authRequest(c, "GET", s.charmsUri(c, ""), "", nil)
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

	resp, err := s.sendRequest(c, machine.Tag(), password, "GET", s.charmsUri(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusUnauthorized, "unauthorized", "")

	// Now try a user login.
	resp, err = s.authRequest(c, "GET", s.charmsUri(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusMethodNotAllowed, `unsupported method: "GET"`, "")
}

func (s *charmsSuite) TestUploadRequiresSeries(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.charmsUri(c, ""), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "expected series= URL argument", "")
}

func (s *charmsSuite) TestUploadRequiresMultipartForm(c *gc.C) {
	resp, err := s.authRequest(c, "POST", s.charmsUri(c, "?series=quantal"), "", nil)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "request Content-Type isn't multipart/form-data", "")
}

func (s *charmsSuite) TestUploadRequiresUploadedFile(c *gc.C) {
	resp, err := s.uploadRequest(c, s.charmsUri(c, "?series=quantal"), false)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "expected a single uploaded file, got none", "")
}

func (s *charmsSuite) TestUploadRequiresSingleUploadedFile(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)
	path := tempFile.Name()

	resp, err := s.uploadRequest(c, s.charmsUri(c, "?series=quantal"), true, path, path)
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "expected a single uploaded file, got more", "")
}

func (s *charmsSuite) TestUploadFailsWithInvalidZip(c *gc.C) {
	// Create an empty file.
	tempFile, err := ioutil.TempFile(c.MkDir(), "charm")
	c.Assert(err, gc.IsNil)

	// Pretend we upload a zip by setting the Content-Type, so we can
	// check the error at extraction time later.
	resp, err := s.uploadRequest(c, s.charmsUri(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	s.assertResponse(c, resp, http.StatusBadRequest, "invalid charm archive: zip: not a valid zip file", "")

	// Now try with the default Content-Type.
	resp, err = s.uploadRequest(c, s.charmsUri(c, "?series=quantal"), false, tempFile.Name())
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
	resp, err := s.uploadRequest(c, s.charmsUri(c, "?series=quantal"), true, ch.Path)
	c.Assert(err, gc.IsNil)
	expectedUrl := charm.MustParseURL("local:quantal/dummy-2")
	s.assertResponse(c, resp, http.StatusOK, "", expectedUrl.String())
	sch, err := s.State.Charm(expectedUrl)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedUrl)
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
	resp, err := s.uploadRequest(c, s.charmsUri(c, "?series=quantal"), true, tempFile.Name())
	c.Assert(err, gc.IsNil)
	expectedUrl := charm.MustParseURL("local:quantal/dummy-123")
	s.assertResponse(c, resp, http.StatusOK, "", expectedUrl.String())
	sch, err := s.State.Charm(expectedUrl)
	c.Assert(err, gc.IsNil)
	c.Assert(sch.URL(), gc.DeepEquals, expectedUrl)
	c.Assert(sch.Revision(), gc.Equals, 123)
	c.Assert(sch.IsUploaded(), jc.IsTrue)

	// Finally, verify the SHA256 and uploaded URL.
	expectedSha256, _, err := getSha256(tempFile)
	c.Assert(err, gc.IsNil)
	name := charm.Quote(expectedUrl.String())
	storage, err := apiserver.GetEnvironStorage(s.State)
	c.Assert(err, gc.IsNil)
	expectedUploadUrl, err := storage.URL(name)
	c.Assert(err, gc.IsNil)

	c.Assert(sch.BundleURL().String(), gc.Equals, expectedUploadUrl)
	c.Assert(sch.BundleSha256(), gc.Equals, expectedSha256)
}

func (s *charmsSuite) charmsUri(c *gc.C, query string) string {
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

func (s *charmsSuite) uploadRequest(c *gc.C, uri string, asZip bool, paths ...string) (*http.Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	prepare := func(i int, path string) error {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		fieldname := fmt.Sprintf("charm%d", i)
		filename := fieldname + ".zip"
		var part io.Writer

		if asZip {
			// The following is copied from mime/multipart
			// Writer.CreateFormFile method, with slight
			// modification to set the correct content type.
			quoteEscaper := strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
			escapeQuotes := quoteEscaper.Replace

			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition",
				fmt.Sprintf("form-data; name=%q; filename=%q",
					escapeQuotes(fieldname), escapeQuotes(filename)))
			h.Set("Content-Type", "application/zip")
			part, err = writer.CreatePart(h)
		} else {
			part, err = writer.CreateFormFile(fieldname, filename)
		}
		if err != nil {
			return err
		}
		if _, err := io.Copy(part, file); err != nil {
			return err
		}
		return nil
	}

	for i, path := range paths {
		if err := prepare(i, path); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return s.authRequest(c, "POST", uri, writer.FormDataContentType(), body)
}

func (s *charmsSuite) assertResponse(c *gc.C, resp *http.Response, expCode int, expError, expCharmURL string) {
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	c.Assert(err, gc.IsNil)
	var jsonResponse apiserver.CharmsResponse
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

func getSha256(source io.ReadSeeker) (string, int64, error) {
	// Rewind the reader to get the accurate hash.
	if _, err := source.Seek(0, 0); err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	size, err := io.Copy(hash, source)
	if err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	return digest, size, nil
}
