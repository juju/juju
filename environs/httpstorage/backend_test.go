// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpstorage_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/httpstorage"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

const testAuthkey = "jabberwocky"

func TestLocal(t *stdtesting.T) {
	gc.TestingT(t)
}

type backendSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&backendSuite{})

// startServer starts a new local storage server
// using a temporary directory and returns the listener,
// a base URL for the server and the directory path.
func startServer(c *gc.C) (listener net.Listener, url, dataDir string) {
	dataDir = c.MkDir()
	embedded, err := filestorage.NewFileStorageWriter(dataDir)
	c.Assert(err, gc.IsNil)
	listener, err = httpstorage.Serve("localhost:0", embedded)
	c.Assert(err, gc.IsNil)
	return listener, fmt.Sprintf("http://%s/", listener.Addr()), dataDir
}

// startServerTLS starts a new TLS-based local storage server
// using a temporary directory and returns the listener,
// a base URL for the server and the directory path.
func startServerTLS(c *gc.C) (listener net.Listener, url, dataDir string) {
	dataDir = c.MkDir()
	embedded, err := filestorage.NewFileStorageWriter(dataDir)
	c.Assert(err, gc.IsNil)
	hostnames := []string{"127.0.0.1"}
	listener, err = httpstorage.ServeTLS(
		"127.0.0.1:0",
		embedded,
		coretesting.CACert,
		coretesting.CAKey,
		hostnames,
		testAuthkey,
	)
	c.Assert(err, gc.IsNil)
	return listener, fmt.Sprintf("http://localhost:%d/", listener.Addr().(*net.TCPAddr).Port), dataDir
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

func (s *backendSuite) TestHeadNonAuth(c *gc.C) {
	// HEAD is unsupported for non-authenticating servers.
	listener, url, _ := startServer(c)
	defer listener.Close()
	resp, err := http.Head(url)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusMethodNotAllowed)
}

func (s *backendSuite) TestHeadAuth(c *gc.C) {
	// HEAD on an authenticating server will return the HTTPS counterpart URL.
	client, url, datadir := s.tlsServerAndClient(c)
	createTestData(c, datadir)

	resp, err := client.Head(url)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	location, err := resp.Location()
	c.Assert(err, gc.IsNil)
	c.Assert(location.String(), gc.Matches, "https://localhost:[0-9]{5}/")
	testGet(c, client, location.String())
}

func (s *backendSuite) TestHeadCustomHost(c *gc.C) {
	// HEAD with a custom "Host:" header; the server should respond
	// with a Location with the specified Host header.
	client, url, _ := s.tlsServerAndClient(c)
	req, err := http.NewRequest("HEAD", url+"arbitrary", nil)
	c.Assert(err, gc.IsNil)
	req.Host = "notarealhost"
	resp, err := client.Do(req)
	c.Assert(err, gc.IsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	location, err := resp.Location()
	c.Assert(err, gc.IsNil)
	c.Assert(location.String(), gc.Matches, "https://notarealhost:[0-9]{5}/arbitrary")
}

func (s *backendSuite) TestGet(c *gc.C) {
	// Test retrieving a file from a storage.
	listener, url, dataDir := startServer(c)
	defer listener.Close()
	createTestData(c, dataDir)
	testGet(c, http.DefaultClient, url)
}

func testGet(c *gc.C, client *http.Client, url string) {
	check := func(tc testCase) {
		resp, err := client.Get(url + tc.name)
		c.Assert(err, gc.IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, gc.Equals, tc.status)
			return
		} else {
			c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		c.Assert(err, gc.IsNil)
		c.Assert(buf.String(), gc.Equals, tc.content)
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

func (s *backendSuite) TestList(c *gc.C) {
	// Test listing file of a storage.
	listener, url, dataDir := startServer(c)
	defer listener.Close()
	createTestData(c, dataDir)
	testList(c, http.DefaultClient, url)
}

func testList(c *gc.C, client *http.Client, url string) {
	check := func(tc testCase) {
		resp, err := client.Get(url + tc.name + "*")
		c.Assert(err, gc.IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, gc.Equals, tc.status)
			return
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		c.Assert(err, gc.IsNil)
		names := strings.Split(buf.String(), "\n")
		c.Assert(names, gc.DeepEquals, tc.found)
	}
	for i, tc := range listTests {
		c.Logf("test %d", i)
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

func (s *backendSuite) TestPut(c *gc.C) {
	// Test sending a file to the storage.
	listener, url, dataDir := startServer(c)
	defer listener.Close()
	createTestData(c, dataDir)
	testPut(c, http.DefaultClient, url, dataDir, true)
}

func testPut(c *gc.C, client *http.Client, url, dataDir string, authorized bool) {
	check := func(tc testCase) {
		req, err := http.NewRequest("PUT", url+tc.name, bytes.NewBufferString(tc.content))
		c.Assert(err, gc.IsNil)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := client.Do(req)
		c.Assert(err, gc.IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, gc.Equals, tc.status)
			return
		} else if !authorized {
			c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
			return
		}
		c.Assert(resp.StatusCode, gc.Equals, http.StatusCreated)

		fp := filepath.Join(dataDir, tc.name)
		b, err := ioutil.ReadFile(fp)
		c.Assert(err, gc.IsNil)
		c.Assert(string(b), gc.Equals, tc.content)
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

func (s *backendSuite) TestRemove(c *gc.C) {
	// Test removing a file in the storage.
	listener, url, dataDir := startServer(c)
	defer listener.Close()
	createTestData(c, dataDir)
	testRemove(c, http.DefaultClient, url, dataDir, true)
}

func testRemove(c *gc.C, client *http.Client, url, dataDir string, authorized bool) {
	check := func(tc testCase) {
		fp := filepath.Join(dataDir, tc.name)
		dir, _ := filepath.Split(fp)
		err := os.MkdirAll(dir, 0777)
		c.Assert(err, gc.IsNil)
		err = ioutil.WriteFile(fp, []byte(tc.content), 0644)
		c.Assert(err, gc.IsNil)

		req, err := http.NewRequest("DELETE", url+tc.name, nil)
		c.Assert(err, gc.IsNil)
		resp, err := client.Do(req)
		c.Assert(err, gc.IsNil)
		if tc.status != 0 {
			c.Assert(resp.StatusCode, gc.Equals, tc.status)
			return
		} else if !authorized {
			c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
			return
		}
		c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)

		_, err = os.Stat(fp)
		c.Assert(os.IsNotExist(err), gc.Equals, true)
	}
	for i, tc := range removeTests {
		c.Logf("test %d", i)
		check(tc)
	}
}

func createTestData(c *gc.C, dataDir string) {
	writeData := func(dir, name, data string) {
		fn := filepath.Join(dir, name)
		c.Logf("writing data to %q", fn)
		err := ioutil.WriteFile(fn, []byte(data), 0644)
		c.Assert(err, gc.IsNil)
	}

	writeData(dataDir, "foo", "this is file 'foo'")
	writeData(dataDir, "bar", "this is file 'bar'")
	writeData(dataDir, "baz", "this is file 'baz'")
	writeData(dataDir, "yadda", "this is file 'yadda'")

	innerDir := filepath.Join(dataDir, "inner")
	err := os.MkdirAll(innerDir, 0777)
	c.Assert(err, gc.IsNil)

	writeData(innerDir, "fooin", "this is inner file 'fooin'")
	writeData(innerDir, "barin", "this is inner file 'barin'")
	writeData(innerDir, "bazin", "this is inner file 'bazin'")
}

func (b *backendSuite) tlsServerAndClient(c *gc.C) (client *http.Client, url, dataDir string) {
	listener, url, dataDir := startServerTLS(c)
	b.AddCleanup(func(*gc.C) { listener.Close() })
	caCerts := x509.NewCertPool()
	c.Assert(caCerts.AppendCertsFromPEM([]byte(coretesting.CACert)), jc.IsTrue)
	client = &http.Client{
		Transport: utils.NewHttpTLSTransport(&tls.Config{RootCAs: caCerts}),
	}
	return client, url, dataDir
}

func (b *backendSuite) TestTLSUnauthenticatedGet(c *gc.C) {
	client, url, dataDir := b.tlsServerAndClient(c)
	createTestData(c, dataDir)
	testGet(c, client, url)
}

func (b *backendSuite) TestTLSUnauthenticatedList(c *gc.C) {
	client, url, dataDir := b.tlsServerAndClient(c)
	createTestData(c, dataDir)
	testList(c, client, url)
}

func (b *backendSuite) TestTLSUnauthenticatedPut(c *gc.C) {
	client, url, dataDir := b.tlsServerAndClient(c)
	createTestData(c, dataDir)
	testPut(c, client, url, dataDir, false)
}

func (b *backendSuite) TestTLSUnauthenticatedRemove(c *gc.C) {
	client, url, dataDir := b.tlsServerAndClient(c)
	createTestData(c, dataDir)
	testRemove(c, client, url, dataDir, false)
}
