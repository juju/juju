// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

// sendJSONResponse encodes the given content as JSON and writes it to the
// given response writer.
func sendJSONResponse(c *gc.C, w http.ResponseWriter, content interface{}) {
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	encoder := json.NewEncoder(w)
	err := encoder.Encode(content)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *clientSuite) TestGUIArchives(c *gc.C) {
	client := s.APIState.Client()
	called := false
	response := params.GUIArchiveResponse{
		Versions: []params.GUIArchiveVersion{{
			Version: version.MustParse("1.0.0"),
			SHA256:  "hash1",
			Current: false,
		}, {
			Version: version.MustParse("2.0.0"),
			SHA256:  "hash2",
			Current: true,
		}},
	}

	// Set up a fake endpoint for tests.
	defer fakeAPIEndpoint(c, client, "/gui-archive", "GET",
		func(w http.ResponseWriter, req *http.Request) {
			defer req.Body.Close()
			called = true
			sendJSONResponse(c, w, response)
		},
	).Close()

	versions, err := client.GUIArchives()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(versions, jc.DeepEquals, response.Versions)
	c.Assert(called, jc.IsTrue)
}

func (s *clientSuite) TestGUIArchivesError(c *gc.C) {
	client := s.APIState.Client()

	// Set up a fake endpoint for tests.
	defer fakeAPIEndpoint(c, client, "/gui-archive", "GET",
		func(w http.ResponseWriter, req *http.Request) {
			defer req.Body.Close()
			w.WriteHeader(http.StatusBadRequest)
		},
	).Close()

	versions, err := client.GUIArchives()
	c.Assert(err, gc.ErrorMatches, "cannot retrieve GUI archives info: .*")
	c.Assert(versions, gc.IsNil)
}

func (s *clientSuite) TestUploadGUIArchive(c *gc.C) {
	client := s.APIState.Client()
	called := false
	archive := []byte("archive content")
	hash, size, vers := "archive-hash", int64(len(archive)), version.MustParse("2.1.0")

	// Set up a fake endpoint for tests.
	defer fakeAPIEndpoint(c, client, "/gui-archive", "POST",
		func(w http.ResponseWriter, req *http.Request) {
			defer req.Body.Close()
			called = true
			err := req.ParseForm()
			c.Assert(err, jc.ErrorIsNil)
			// Check version and content length.
			c.Assert(req.Form.Get("version"), gc.Equals, vers.String())
			c.Assert(req.ContentLength, gc.Equals, size)
			// Check request body.
			obtainedArchive, err := ioutil.ReadAll(req.Body)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(obtainedArchive, gc.DeepEquals, archive)
			// Check hash.
			h := req.Form.Get("hash")
			c.Assert(h, gc.Equals, hash)
			// Send the response.
			sendJSONResponse(c, w, params.GUIArchiveVersion{
				Current: true,
			})
		},
	).Close()

	current, err := client.UploadGUIArchive(bytes.NewReader(archive), hash, size, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, jc.IsTrue)
	c.Assert(called, jc.IsTrue)
}

func (s *clientSuite) TestUploadGUIArchiveError(c *gc.C) {
	client := s.APIState.Client()
	archive := []byte("archive content")
	hash, size, vers := "archive-hash", int64(len(archive)), version.MustParse("2.1.0")

	// Set up a fake endpoint for tests.
	defer fakeAPIEndpoint(c, client, "/gui-archive", "POST",
		func(w http.ResponseWriter, req *http.Request) {
			defer req.Body.Close()
			w.WriteHeader(http.StatusBadRequest)
		},
	).Close()

	current, err := client.UploadGUIArchive(bytes.NewReader(archive), hash, size, vers)
	c.Assert(err, gc.ErrorMatches, "cannot upload the GUI archive: .*")
	c.Assert(current, jc.IsFalse)
}

func (s *clientSuite) TestSelectGUIVersion(c *gc.C) {
	client := s.APIState.Client()
	called := false
	vers := version.MustParse("2.0.42")

	// Set up a fake endpoint for tests.
	defer fakeAPIEndpoint(c, client, "/gui-version", "PUT",
		func(w http.ResponseWriter, req *http.Request) {
			defer req.Body.Close()
			called = true
			// Check request body.
			var request params.GUIVersionRequest
			decoder := json.NewDecoder(req.Body)
			err := decoder.Decode(&request)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(request.Version, gc.Equals, vers)
		},
	).Close()

	err := client.SelectGUIVersion(vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *clientSuite) TestSelectGUIVersionError(c *gc.C) {
	client := s.APIState.Client()
	vers := version.MustParse("2.0.42")

	// Set up a fake endpoint for tests.
	defer fakeAPIEndpoint(c, client, "/gui-version", "PUT",
		func(w http.ResponseWriter, req *http.Request) {
			defer req.Body.Close()
			w.WriteHeader(http.StatusBadRequest)
		},
	).Close()

	err := client.SelectGUIVersion(vers)
	c.Assert(err, gc.ErrorMatches, "cannot select GUI version: .*")
}
