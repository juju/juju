// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/local"
)

type backendSuite struct{}

var _ = Suite(&backendSuite{})

var testSetMu sync.Mutex

// nextTestSet returns a new port number, listener and data directory.
func nextTestSet(c *C) (int, net.Listener, string) {
	testSetMu.Lock()
	defer testSetMu.Unlock()

	dataDir := c.MkDir()
	listener, err := local.Listen(dataDir, "127.0.0.1", 0)
	c.Assert(err, IsNil)
	port := listener.Addr().(*net.TCPAddr).Port

	return port, listener, dataDir
}

type testCase struct {
	name    string
	content string
	found   []string
	status  int
}

var getTests = []testCase{
	{
		// Get existing file.
		name:    "foo",
		content: "this is file 'foo'",
	},
	{
		// Get existing file.
		name:    "bar",
		content: "this is file 'bar'",
	},
	{
		// Get existing file.
		name:    "baz",
		content: "this is file 'baz'",
	},
	{
		// Get existing file.
		name:    "yadda",
		content: "this is file 'yadda'",
	},
	{
		// Get existing file from nested directory.
		name:    "inner/fooin",
		content: "this is inner file 'fooin'",
	},
	{
		// Get existing file from nested directory.
		name:    "inner/barin",
		content: "this is inner file 'barin'",
	},
	{
		// Get non-existing file.
		name:   "dummy",
		status: 404,
	},
	{
		// Get non-existing file from nested directory.
		name:   "inner/dummy",
		status: 404,
	},
	{
		// Get with a relative path ".." based on the
		// root is passed without invoking the handler
		// function.
		name:   "../dummy",
		status: 404,
	},
	{
		// Get with a relative path ".." based on the
		// root is passed without invoking the handler
		// function.
		name:    "../foo",
		content: "this is file 'foo'",
	},
	{
		// Get on a directory returns a 404 as it is
		// not a file.
		name:   "inner",
		status: 404,
	},
}

func (s *backendSuite) TestGet(c *C) {
	// Test retrieving a file from a storage.
	portNo, listener, dataDir := nextTestSet(c)
	defer listener.Close()

	createTestData(c, dataDir)

	check := func(tc testCase) {
		url := fmt.Sprintf("http://localhost:%d/%s", portNo, tc.name)
		resp, err := http.Get(url)
		c.Assert(err, IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, Equals, tc.status)
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		c.Assert(err, IsNil)
		c.Assert(buf.String(), Equals, tc.content)
	}
	for _, tc := range getTests {
		check(tc)
	}
}

var listTests = []testCase{
	{
		// List with a full filename.
		name:  "foo",
		found: []string{"foo"},
	},
	{
		// List with a name matching two files.
		name:  "ba",
		found: []string{"bar", "baz"},
	},
	{
		// List the contents of a directory.
		name:  "inner/",
		found: []string{"inner/barin", "inner/bazin", "inner/fooin"},
	},
	{
		// List with a name matching two files in
		// a directory.
		name:  "inner/ba",
		found: []string{"inner/barin", "inner/bazin"},
	},
	{
		// List with no name also lists the contents of all
		// directories.
		name:  "",
		found: []string{"bar", "baz", "foo", "inner/barin", "inner/bazin", "inner/fooin", "yadda"},
	},
	{
		// List with a non-matching name returns an empty
		// body which is evaluated to a slice with an empty
		// string in the test (simplification).
		name:  "zzz",
		found: []string{""},
	},
	{
		// List with a relative path ".." based on the
		// root is passed without invoking the handler
		// function. So returns the contents of all
		// directories.
		name:  "../",
		found: []string{"bar", "baz", "foo", "inner/barin", "inner/bazin", "inner/fooin", "yadda"},
	},
}

func (s *backendSuite) TestList(c *C) {
	// Test listing file of a storage.
	portNo, listener, dataDir := nextTestSet(c)
	defer listener.Close()

	createTestData(c, dataDir)

	check := func(tc testCase) {
		url := fmt.Sprintf("http://localhost:%d/%s*", portNo, tc.name)
		resp, err := http.Get(url)
		c.Assert(err, IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, Equals, tc.status)
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		c.Assert(err, IsNil)
		names := strings.Split(buf.String(), "\n")
		c.Assert(names, DeepEquals, tc.found)
	}
	for _, tc := range listTests {
		check(tc)
	}
}

var putTests = []testCase{
	{
		// Put a file in the root directory.
		name:    "porterhouse",
		content: "this is the sent file 'porterhouse'",
	},
	{
		// Put a file with a relative path ".." is resolved
		// a redirect 301 by the Go HTTP daemon. The handler
		// isn't aware of it.
		name:   "../no-way",
		status: 301,
	},
	{
		// Put a file in a nested directory.
		name:    "deep/cambridge",
		content: "this is the sent file 'deep/cambridge'",
	},
}

func (s *backendSuite) TestPut(c *C) {
	// Test sending a file to the storage.
	portNo, listener, dataDir := nextTestSet(c)
	defer listener.Close()

	createTestData(c, dataDir)

	check := func(tc testCase) {
		url := fmt.Sprintf("http://localhost:%d/%s", portNo, tc.name)
		req, err := http.NewRequest("PUT", url, bytes.NewBufferString(tc.content))
		c.Assert(err, IsNil)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		c.Assert(err, IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, Equals, tc.status)
			return
		}
		c.Assert(resp.StatusCode, Equals, 201)

		fp := filepath.Join(dataDir, tc.name)
		b, err := ioutil.ReadFile(fp)
		c.Assert(err, IsNil)
		c.Assert(string(b), Equals, tc.content)
	}
	for _, tc := range putTests {
		check(tc)
	}
}

var removeTests = []testCase{
	{
		// Delete a file in the root directory.
		name:    "fox",
		content: "the quick brown fox jumps over the lazy dog",
	},
	{
		// Delete a file in a nested directory.
		name:    "quick/brown/fox",
		content: "the quick brown fox jumps over the lazy dog",
	},
	{
		// Delete a non-existing file leads to no error.
		name: "dog",
	},
	{
		// Delete a file with a relative path ".." is resolved
		// a redirect 301 by the Go HTTP daemon. The handler
		// doesn't get aware of it.
		name:   "../something",
		status: 301,
	},
}

func (s *backendSuite) TestRemove(c *C) {
	// Test removing a file in the storage.
	portNo, listener, dataDir := nextTestSet(c)
	defer listener.Close()

	createTestData(c, dataDir)

	check := func(tc testCase) {
		fp := filepath.Join(dataDir, tc.name)
		dir, _ := filepath.Split(fp)
		err := os.MkdirAll(dir, 0777)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(fp, []byte(tc.content), 0644)
		c.Assert(err, IsNil)

		url := fmt.Sprintf("http://localhost:%d/%s", portNo, tc.name)
		req, err := http.NewRequest("DELETE", url, nil)
		c.Assert(err, IsNil)
		resp, err := http.DefaultClient.Do(req)
		c.Assert(err, IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, Equals, tc.status)
			return
		}
		c.Assert(resp.StatusCode, Equals, 200)

		_, err = os.Stat(fp)
		c.Assert(err, ErrorMatches, ".*: no such file or directory")
	}
	for _, tc := range removeTests {
		check(tc)
	}
}

func createTestData(c *C, dataDir string) {
	writeData := func(dir, name, data string) {
		fn := filepath.Join(dir, name)
		err := ioutil.WriteFile(fn, []byte(data), 0644)
		c.Assert(err, IsNil)
	}

	dir := filepath.Join(dataDir)

	writeData(dir, "foo", "this is file 'foo'")
	writeData(dir, "bar", "this is file 'bar'")
	writeData(dir, "baz", "this is file 'baz'")
	writeData(dir, "yadda", "this is file 'yadda'")

	dir = filepath.Join(dataDir, "inner")
	err := os.MkdirAll(dir, 0777)
	c.Assert(err, IsNil)

	writeData(dir, "fooin", "this is inner file 'fooin'")
	writeData(dir, "barin", "this is inner file 'barin'")
	writeData(dir, "bazin", "this is inner file 'bazin'")
}
