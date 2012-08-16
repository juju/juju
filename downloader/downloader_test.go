package downloader_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/testing"
	"net"
	"net/http"
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
	l := newServer()
	defer l.close()

	content := l.addContent("/archive.tgz", "archive")
	d := downloader.New(content.url)
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
	l := newServer()
	defer l.close()
	// Add some content, then delete it - we should
	// get a 404 response.
	url := l.addContent("/archive.tgz", "archive").url
	delete(l.contents, "/archive.tgz")
	d := downloader.New(url)
	status := <-d.Done()
	c.Assert(status.File, IsNil)
	c.Assert(status.Err, ErrorMatches, `cannot download ".*": bad http response: 404 Not Found`)
}

func (s *suite) TestStopDownload(c *C) {
	l := newServer()
	defer l.close()
	content := l.addContent("/x.tgz", "content")
	d := downloader.New(content.url)
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

type content struct {
	url  string
	data []byte
}

type server struct {
	l        net.Listener
	contents map[string]*content
}

func newServer() *server {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Errorf("cannot start server: %v", err))
	}
	srv := &server{l, make(map[string]*content)}
	go http.Serve(l, srv)
	return srv
}

func (srv *server) close() {
	srv.l.Close()
}

// addContent makes the given data available from the server
// at the given URL path.
func (srv *server) addContent(path string, data string) *content {
	c := &content{
		data: []byte(data),
	}
	c.url = fmt.Sprintf("http://%v%s", srv.l.Addr(), path)
	srv.contents[path] = c
	return c
}

func (srv *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := srv.contents[req.URL.Path]
	if c == nil {
		http.NotFound(w, req)
		return
	}
	w.Write(c.data)
}
