// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type syncToolsSuite struct {
	testing.LoggingSuite
	home         *testing.FakeHome
	targetEnv    environs.Environ
	origVersion  version.Binary
	origLocation string
	listener     net.Listener
	storage      *testStorage
}

func (s *syncToolsSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.origVersion = version.Current
	// It's important that this be v1 to match the test data.
	version.Current.Number = version.MustParse("1.2.3")

	// Create a target environments.yaml and make sure its environment is empty.
	s.home = testing.MakeFakeHome(c, `
environments:
    test-target:
        type: dummy
        state-server: false
        authorized-keys: "not-really-one"
`)
	var err error
	s.targetEnv, err = environs.NewFromName("test-target")
	c.Assert(err, IsNil)
	envtesting.RemoveAllTools(c, s.targetEnv)

	// Create a source environment and populate its public tools.
	s.listener, s.storage, err = listen("127.0.0.1", 0)
	c.Assert(err, IsNil)

	for _, vers := range vAll {
		s.storage.putFakeToolsVersion(vers)
	}

	s.origLocation = defaultToolsLocation
	defaultToolsLocation = s.storage.location
}

func (s *syncToolsSuite) TearDownTest(c *C) {
	c.Assert(s.listener.Close(), IsNil)
	defaultToolsLocation = s.origLocation
	dummy.Reset()
	s.home.Restore()
	version.Current = s.origVersion
	s.LoggingSuite.TearDownTest(c)
}

var _ = Suite(&syncToolsSuite{})

func runSyncToolsCommand(c *C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, &SyncToolsCommand{}, args)
}

func (s *syncToolsSuite) TestHelp(c *C) {
	ctx, err := runSyncToolsCommand(c, "-h")
	c.Assert(err, ErrorMatches, "flag: help requested")
	c.Assert(ctx, IsNil)
}

func assertToolsList(c *C, list tools.List, expected []version.Binary) {
	urls := list.URLs()
	c.Check(urls, HasLen, len(expected))
	for _, vers := range expected {
		c.Assert(urls[vers], Not(Equals), "")
	}
}

func assertEmpty(c *C, storage environs.StorageReader) {
	list, err := tools.ReadList(storage, 1)
	if len(list) > 0 {
		c.Logf("got unexpected tools: %s", list)
	}
	c.Assert(err, Equals, tools.ErrNoTools)
}

func (s *syncToolsSuite) TestCopyNewestFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyNewestDevFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--dev")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest v1 dev tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v190all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyAllFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// All released v1 tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyAllDevFromDummy(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--all", "--dev")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// All v1 tools, dev and release, made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v1all)

	// Public bucket was not touched.
	assertEmpty(c, s.targetEnv.PublicStorage())
}

func (s *syncToolsSuite) TestCopyToDummyPublic(c *C) {
	ctx, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, IsNil)
	c.Assert(ctx, NotNil)

	// Newest released tools made available to target env.
	targetTools, err := environs.FindAvailableTools(s.targetEnv, 1)
	c.Assert(err, IsNil)
	assertToolsList(c, targetTools, v100all)

	// Private bucket was not touched.
	assertEmpty(c, s.targetEnv.Storage())
}

func (s *syncToolsSuite) TestCopyToDummyPublicBlockedByPrivate(c *C) {
	envtesting.UploadFakeToolsVersion(c, s.targetEnv.Storage(), v200p64)

	_, err := runSyncToolsCommand(c, "-e", "test-target", "--public")
	c.Assert(err, ErrorMatches, "private tools present: public tools would be ignored")
	assertEmpty(c, s.targetEnv.PublicStorage())
}

var (
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100all = []version.Binary{v100p64, v100q64, v100q32}
	v190q64 = version.MustParseBinary("1.9.0-quantal-amd64")
	v190p32 = version.MustParseBinary("1.9.0-precise-i386")
	v190all = []version.Binary{v190q64, v190p32}
	v1all   = append(v100all, v190all...)
	v200p64 = version.MustParseBinary("2.0.0-precise-amd64")
	vAll    = append(v1all, v200p64)
)

// testStorage acts like the juju distribution storage at S3 
// to provide the juju tools.
type testStorage struct {
	location string
	files    map[string][]byte
}

// ServeHTTP handles the HTTP requests to the storage mock.
func (t *testStorage) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		if req.URL.Path == "/" {
			t.handleIndex(w, req)
		} else {
			t.handleGet(w, req)
		}
	default:
		http.Error(w, "method "+req.Method+" is not supported", http.StatusMethodNotAllowed)
	}
}

// handleIndex returns the index XML file to the client.
func (t *testStorage) handleIndex(w http.ResponseWriter, req *http.Request) {
	lbr := &listBucketResult{
		Name:        "juju-dist",
		Prefix:      "",
		Marker:      "",
		MaxKeys:     1000,
		IsTruncated: false,
	}
	names := []string{}
	for name := range t.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := crc32.NewIEEE()
		h.Write([]byte(t.files[name]))
		contents := &contents{
			Key:          name,
			LastModified: time.Now(),
			ETag:         fmt.Sprintf("%x", h.Sum(nil)),
			Size:         len([]byte(t.files[name])),
			StorageClass: "STANDARD",
		}
		lbr.Contents = append(lbr.Contents, contents)
	}
	buf, err := xml.Marshal(lbr)
	if err != nil {
		http.Error(w, fmt.Sprintf("500 %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Write(buf)
}

// handleGet returns a storage file to the client.
func (t *testStorage) handleGet(w http.ResponseWriter, req *http.Request) {
	data, ok := t.files[req.URL.Path]
	if !ok {
		http.Error(w, "404 file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(data)
}

// putFakeToolsVersion stores a faked binary in the tools storage.
func (t *testStorage) putFakeToolsVersion(vers version.Binary) {
	data := vers.String()
	name := tools.StorageName(vers)
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		// Also create paths as entries. Needed for
		// the correct contents of the list bucket result.
		path := ""
		for i := 0; i < len(parts)-1; i++ {
			path = path + parts[i] + "/"
			t.files[path] = []byte{}
		}
	}
	t.files[name] = []byte(data)
}

// listen starts an HTTP listener to serve the
// provider storage.
func listen(ip string, port int) (net.Listener, *testStorage, error) {
	storage := &testStorage{
		files: make(map[string][]byte),
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot start listener: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", storage)

	go http.Serve(listener, mux)

	storage.location = fmt.Sprintf("http://%s:%d/", ip, listener.Addr().(*net.TCPAddr).Port)

	return listener, storage, nil
}
