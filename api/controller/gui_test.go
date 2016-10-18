// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller"
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

// withHTTPClient sets up a fixture with the given address and handle, then
// runs the given test and checks that the HTTP handler has been called with
// the given method.
func withHTTPClient(c *gc.C, address, expectMethod string, handle func(http.ResponseWriter, *http.Request), test func(*controller.Client)) {
	fix := newHTTPFixture(address, handle)
	stub := fix.run(c, func(ac base.APICallCloser) {
		client := controller.NewClient(ac)
		test(client)
	})
	stub.CheckCalls(c, []testing.StubCall{{expectMethod, nil}})
}

func (s *Suite) TestGUIArchives(c *gc.C) {
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
	withHTTPClient(c, "/gui-archive", "GET", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		sendJSONResponse(c, w, response)
	}, func(client *controller.Client) {
		// Retrieve the GUI archive versions.
		versions, err := client.GUIArchives()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(versions, jc.DeepEquals, response.Versions)
	})
}

func (s *Suite) TestGUIArchivesError(c *gc.C) {
	withHTTPClient(c, "/gui-archive", "GET", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		w.WriteHeader(http.StatusBadRequest)
	}, func(client *controller.Client) {
		// Call to get GUI archive versions.
		versions, err := client.GUIArchives()
		c.Assert(err, gc.ErrorMatches, "cannot retrieve GUI archives info: .*")
		c.Assert(versions, gc.IsNil)
	})
}

func (s *Suite) TestUploadGUIArchive(c *gc.C) {
	archive := []byte("archive content")
	hash, size, vers := "archive-hash", int64(len(archive)), version.MustParse("2.1.0")
	withHTTPClient(c, "/gui-archive", "POST", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
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
	}, func(client *controller.Client) {
		// Upload a new Juju GUI archive.
		current, err := client.UploadGUIArchive(bytes.NewReader(archive), hash, size, vers)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(current, jc.IsTrue)
	})
}

func (s *Suite) TestUploadGUIArchiveError(c *gc.C) {
	archive := []byte("archive content")
	hash, size, vers := "archive-hash", int64(len(archive)), version.MustParse("2.1.0")
	withHTTPClient(c, "/gui-archive", "POST", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		w.WriteHeader(http.StatusBadRequest)
	}, func(client *controller.Client) {
		// Call to upload a new Juju GUI archive.
		current, err := client.UploadGUIArchive(bytes.NewReader(archive), hash, size, vers)
		c.Assert(err, gc.ErrorMatches, "cannot upload the GUI archive: .*")
		c.Assert(current, jc.IsFalse)
	})
}

func (s *Suite) TestSelectGUIVersion(c *gc.C) {
	vers := version.MustParse("2.0.42")
	withHTTPClient(c, "/gui-version", "PUT", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		// Check request body.
		var request params.GUIVersionRequest
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&request)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(request.Version, gc.Equals, vers)
	}, func(client *controller.Client) {
		// Switch to a specific Juju GUI version.
		err := client.SelectGUIVersion(vers)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *Suite) TestSelectGUIVersionError(c *gc.C) {
	vers := version.MustParse("2.0.42")
	withHTTPClient(c, "/gui-version", "PUT", func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		w.WriteHeader(http.StatusBadRequest)
	}, func(client *controller.Client) {
		// Call to select a Juju GUI version.
		err := client.SelectGUIVersion(vers)
		c.Assert(err, gc.ErrorMatches, "cannot select GUI version: .*")
	})
}
