package downloader_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	stdtesting "testing"
	"time"
)

type suite struct {
	testing.LoggingSuite
}

var _ = Suite(&suite{})

func Test(t *stdtesting.T) {
	TestingT(t)
}

func (s *suite) SetUpTest(c *C) {
	downloader.TempDir = c.MkDir()
}

func (s *suite) TearDownTest(c *C) {
	downloader.TempDir = os.TempDir()
}

func (s *suite) TestDownload(c *C) {
	l := testing.NewServer()
	defer l.Close()

	url := l.AddContent("/archive.tgz", []byte("archive"))
	d := downloader.New(url)
	status := <-d.Done()
	c.Assert(status.Err, IsNil)
	c.Assert(status.File, NotNil)
	defer os.Remove(status.File.Name())
	defer status.File.Close()

	dir, _ := filepath.Split(status.File.Name())
	c.Assert(filepath.Clean(dir), Equals, downloader.TempDir)
	assertFileContents(c, status.File, "archive")
}

func (s *suite) TestDownloadError(c *C) {
	l := testing.NewServer()
	defer l.Close()
	// Add some content, then delete it - we should
	// get a 404 response.
	url := l.AddContent("/archive.tgz", nil)
	l.RemoveContent("/archive.tgz")
	d := downloader.New(url)
	status := <-d.Done()
	c.Assert(status.File, IsNil)
	c.Assert(status.Err, ErrorMatches, `cannot download ".*": bad http response: 404 Not Found`)
}

func (s *suite) TestStopDownload(c *C) {
	l := testing.NewServer()
	defer l.Close()
	url := l.AddContent("/x.tgz", []byte("content"))
	d := downloader.New(url)
	d.Stop()
	select {
	case status := <-d.Done():
		c.Fatalf("received status %#v after stop", status)
	case <-time.After(100 * time.Millisecond):
	}
	infos, err := ioutil.ReadDir(downloader.TempDir)
	c.Assert(err, IsNil)
	c.Assert(infos, HasLen, 0)
}

func assertFileContents(c *C, f *os.File, expect string) {
	got, err := ioutil.ReadAll(f)
	c.Assert(err, IsNil)
	if !c.Check(string(got), Equals, expect) {
		info, err := f.Stat()
		c.Assert(err, IsNil)
		c.Logf("info %#v", info)
	}
}
