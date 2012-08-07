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
	stdtesting "testing"
	"time"
)

type suite struct {
	downloader *downloader.Downloader
	dir        string
	testing.LoggingSuite
}

var _ = Suite(&suite{})

func (s *suite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.downloader = downloader.New()
	s.dir = c.MkDir()
}

func (s *suite) TearDownTest(c *C) {
	s.downloader.Stop()
	s.LoggingSuite.TearDownTest(c)
}

func Test(t *stdtesting.T) {
	TestingT(t)
}

func (s *suite) TestDownloader(c *C) {
	l := newServer()
	defer l.close()

	content := l.addContent("/archive.tgz", "archive")
	s.downloader.Start(content.url)
	status := <-s.downloader.Done()
	c.Assert(status.Err, IsNil)
	c.Assert(status.URL, Equals, content.url)
	c.Assert(status.File, NotNil)
	defer os.Remove(status.File.Name())
	defer status.File.Close()
	assertFileContents(c, status.File, "archive")
}

func (s *suite) TestInterruptDownload(c *C) {
	l := newServer()
	defer l.close()
	content1 := l.addContent("/x.tgz", "content1")
	content1.started = make(chan bool)

	content2 := l.addContent("/y.tgz", "content2")

	s.downloader.Start(content1.url)
	<-content1.started

	// Currently we can't interrupt the http get, so we
	// check to make sure that the Start is blocking while
	// the previous download completes.
	// When we fix this, this test will need to change.
	startDone := make(chan bool)
	go func() {
		s.downloader.Start(content2.url)
		startDone <- true
	}()
	select {
	case <-startDone:
		c.Fatalf("Start did not wait for previous download to complete")
	case <-time.After(100 * time.Millisecond):
	}
	// start content1 http get going again.
	content1.started <- true
	status := <-s.downloader.Done()
	c.Check(status.URL, Equals, content2.url)
	c.Check(status.Err, IsNil)
	c.Assert(status.File, NotNil)
	defer os.Remove(status.File.Name())
	defer status.File.Close()
	assertFileContents(c, status.File, "content2")
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
	url     string
	data    []byte
	started chan bool
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
	if c.started != nil {
		// The test wants to know when we have started,
		// so send half the data (meaning that at
		// least *some* of the archive might have been written)
		// then notify that we've started.
		n := len(c.data) / 2
		w.Write(c.data[0:n])
		c.started <- true
		<-c.started
		w.Write(c.data[n:])
	} else {
		w.Write(c.data)
	}
}
