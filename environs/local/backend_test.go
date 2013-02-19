package local_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"launchpad.net/juju-core/environs/local"
)

func TestLocal(t *testing.T) {
	TestingT(t)
}

type backendSuite struct {
	listener net.Listener
	dataDir  string
}

var _ = Suite(&backendSuite{})

const environName = "test-environ"

func (s *backendSuite) SetUpSuite(c *C) {
	var err error
	s.dataDir = c.MkDir()
	s.listener, err = local.Listen(s.dataDir, environName, "127.0.0.1", 60006)
	c.Assert(err, IsNil)

	createTestData(c, s.dataDir)
}

func (s *backendSuite) TearDownSuite(c *C) {
	s.listener.Close()
}

type getTest struct {
	name    string
	content string
	status  int
}

var getTests = []getTest{
	{
		name:    "foo",
		content: "this is file 'foo'",
	},
	{
		name:    "bar",
		content: "this is file 'bar'",
	},
	{
		name:    "baz",
		content: "this is file 'baz'",
	},
	{
		name:    "yadda",
		content: "this is file 'yadda'",
	},
	{
		name:   "dummy",
		status: 404,
	},
	{
		name:   "../dummy",
		status: 404,
	},
}

func (s *backendSuite) TestGet(c *C) {
	// Test retrieving a file from a storage.
	check := func(gt getTest) {
		url := fmt.Sprintf("http://localhost:60006/%s/%s", environName, gt.name)
		resp, err := http.Get(url)
		if gt.status != 0 {
			c.Assert(resp.StatusCode, Equals, gt.status)
			return
		}
		c.Assert(err, IsNil)
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		c.Assert(err, IsNil)
		c.Assert(buf.String(), Equals, gt.content)
	}
	for _, gt := range getTests {
		check(gt)
	}
}

type listTest struct {
	prefix string
	found  []string
	status int
}

var listTests = []listTest{
	{
		prefix: "foo",
		found:  []string{"foo"},
	},
	{
		prefix: "ba",
		found:  []string{"bar", "baz"},
	},
	{
		prefix: "",
		found:  []string{"bar", "baz", "foo", "yadda"},
	},
	{
		prefix: "inner/ba",
		found:  []string{"inner/bar", "inner/baz"},
	},
	{
		prefix: "inner/",
		found:  []string{"inner/bar", "inner/baz", "inner/foo", "inner/yadda"},
	},
	{
		prefix: "zzz",
		found:  []string{""},
	},
	{
		prefix: "../",
		status: 404,
	},
}

func (s *backendSuite) TestList(c *C) {
	// Test listing file of a storage.
	check := func(lt listTest) {
		url := fmt.Sprintf("http://localhost:60006/%s/%s*", environName, lt.prefix)
		resp, err := http.Get(url)
		if lt.status != 0 {
			c.Assert(resp.StatusCode, Equals, lt.status)
			return
		}
		c.Assert(err, IsNil)
		defer resp.Body.Close()
		var buf bytes.Buffer
		_, err = buf.ReadFrom(resp.Body)
		c.Assert(err, IsNil)
		names := strings.Split(buf.String(), "\n")
		c.Assert(names, DeepEquals, lt.found)
	}
	for _, lt := range listTests {
		check(lt)
	}
}

type putTest struct {
	name    string
	content string
	status  int
}

var putTests = []putTest{
	{
		name:    "porterhouse",
		content: "this is the sent file 'porterhouse'",
	},
	{
		name:   "../no-way",
		status: 301,
	},
	{
		name:    "deep/cambridge",
		content: "this is the sent file 'deep/cambridge'",
	},
}

func (s *backendSuite) TestPut(c *C) {
	// Test sending a file to the storage.
	check := func(pt putTest) {
		url := fmt.Sprintf("http://localhost:60006/%s/%s", environName, pt.name)
		req, err := http.NewRequest("PUT", url, bytes.NewBufferString(pt.content))
		c.Assert(err, IsNil)
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if pt.status != 0 {
			c.Assert(resp.StatusCode, Equals, pt.status)
			return
		}
		c.Assert(err, IsNil)
		c.Assert(resp.StatusCode, Equals, 201)

		fp := filepath.Join(s.dataDir, environName, pt.name)
		b, err := ioutil.ReadFile(fp)
		c.Assert(err, IsNil)
		c.Assert(string(b), Equals, pt.content)
	}
	for _, pt := range putTests {
		check(pt)
	}
}

type removeTest struct {
	name    string
	content string
}

var removeTests = []removeTest{
	{
		name:    "fox",
		content: "the quick brown fox jumps over the lazy dog",
	},
	{
		name:    "quick/brown/fox",
		content: "the quick brown fox jumps over the lazy dog",
	},
	{
		name: "dog",
	},
}

func (s *backendSuite) TestRemove(c *C) {
	// Test removing a file in the storage.
	check := func(rt removeTest) {
		fp := filepath.Join(s.dataDir, environName, rt.name)
		dir, _ := filepath.Split(fp)
		err := os.MkdirAll(dir, 0777)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(fp, []byte(rt.content), 0644)
		c.Assert(err, IsNil)

		url := fmt.Sprintf("http://localhost:60006/%s/%s", environName, rt.name)
		req, err := http.NewRequest("DELETE", url, nil)
		c.Assert(err, IsNil)
		resp, err := http.DefaultClient.Do(req)
		c.Assert(err, IsNil)
		c.Assert(resp.StatusCode, Equals, 200)

		_, err = os.Stat(fp)
		c.Assert(err, ErrorMatches, ".*: no such file or directory")
	}
	for _, rt := range removeTests {
		check(rt)
	}
}

func createTestData(c *C, dataDir string) {
	writeData := func(dir, name, data string) {
		fn := filepath.Join(dir, name)
		err := ioutil.WriteFile(fn, []byte(data), 0644)
		c.Assert(err, IsNil)
	}

	dir := filepath.Join(dataDir, environName)

	writeData(dir, "foo", "this is file 'foo'")
	writeData(dir, "bar", "this is file 'bar'")
	writeData(dir, "baz", "this is file 'baz'")
	writeData(dir, "yadda", "this is file 'yadda'")

	dir = filepath.Join(dataDir, environName, "inner")
	err := os.MkdirAll(dir, 0777)
	c.Assert(err, IsNil)

	writeData(dir, "foo", "this is inner file 'foo'")
	writeData(dir, "bar", "this is inner file 'bar'")
	writeData(dir, "baz", "this is inner file 'baz'")
	writeData(dir, "yadda", "this is inner file 'yadda'")
}
