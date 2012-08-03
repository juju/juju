package downloader_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/downloader"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

type suite struct {
	downloader *downloader.Downloader
	dir        string
}

var _ = Suite(&suite{})

func (s *suite) SetUpTest(c *C) {
	s.downloader = downloader.New()
	s.dir = c.MkDir()
}

func (s *suite) TearDownTest(c *C) {
	s.downloader.Stop()
}

func Test(t *testing.T) {
	TestingT(t)
}

//archive prepared using tar
//
//
//download when archive is corrupt in some way
//download when the url can't be fetched
//
//	prepare url
//	downloader.Start
//	downloader.Start
//	start url going
//	
//archive containing:
//	prescribed contents
//	
//
//wait connection to be closed.
//wait for signal

func (s *suite) TestDownloader(c *C) {
	l := newServer()
	defer l.close()
	files := []*file{
		newFile("bar", tar.TypeReg, "bar contents"),
		newFile("foo", tar.TypeReg, "foo contents"),
	}

	content := l.addContent("/archive.tgz", makeArchive(files...))
	dest := filepath.Join(s.dir, "dest")
	s.downloader.Start(content.url, dest)
	status := <-s.downloader.Done()
	c.Assert(status.Error, IsNil)
	c.Assert(status.Dir, Equals, dest)
	c.Assert(status.URL, Equals, content.url)
	assertDirContents(c, dest, content.url, files)
	assertDirNames(c, s.dir, []string{"dest"})
}

// gzyesses holds the result of running:
// yes | head -17000 | gzip
var gzyesses = []byte{
	0x1f, 0x8b, 0x08, 0x00, 0x29, 0xae, 0x1a, 0x50,
	0x00, 0x03, 0xed, 0xc2, 0x31, 0x0d, 0x00, 0x00,
	0x00, 0x02, 0xa0, 0xdf, 0xc6, 0xb6, 0xb7, 0x87,
	0x63, 0xd0, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x38, 0x31, 0x53, 0xad, 0x03,
	0x8d, 0xd0, 0x84, 0x00, 0x00,
}

var badDownloadDataTests = []struct {
	data []byte
	err  string
}{
	{
		makeArchive(newFile("bar", tar.TypeDir, "")),
		"bad file type.*",
	},
	{
		makeArchive(newFile("../../etc/passwd", tar.TypeReg, "")),
		"bad name.*",
	},
	{
		makeArchive(newFile(`\ini.sys`, tar.TypeReg, "")),
		"bad name.*",
	},
	{
		[]byte("x"),
		"unexpected EOF",
	},
	{
		gzyesses,
		"archive/tar: invalid tar header",
	},
}

func (s *suite) TestBadDownloadData(c *C) {
	l := newServer()
	defer l.close()
	content := l.addContent("/x.tgz", nil)
	dest := filepath.Join(s.dir, "dest")
	for i, t := range badDownloadDataTests {
		c.Logf("test %d", i)
		content.data = t.data
		s.downloader.Start(content.url, dest)
		status := <-s.downloader.Done()
		c.Assert(status.Dir, Equals, dest)
		c.Assert(status.URL, Equals, content.url)
		c.Assert(status.Error, ErrorMatches, `cannot download ".*" to ".*": `+t.err)
	}
}

func (s *suite) TestDownloadTwice(c *C) {
	l := newServer()
	files := []*file{
		newFile("test", tar.TypeReg, "test contents"),
	}
	content := l.addContent("/test", makeArchive(files...))
	dest := filepath.Join(s.dir, "dest")
	s.downloader.Start(content.url, dest)
	status := <-s.downloader.Done()
	c.Assert(status.Error, IsNil)

	// Stop the web server and check that
	// a download to the same directory succeeds
	// because the directory aleady exists.
	l.close()
	s.downloader.Start(content.url, dest)
	status = <-s.downloader.Done()
	c.Assert(status.Error, IsNil)
	assertDirContents(c, dest, content.url, files)
}

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(rand.Int31n(256))
	}
	return string(b)
}

func (s *suite) TestInterruptDownload(c *C) {
	l := newServer()
	defer l.close()
	content1 := l.addContent("/x.tgz", makeArchive(
		newFile("juju", tar.TypeReg, "juju exe\n"+randomString(10000)),
		newFile("jujuc", tar.TypeReg, "jujuc exe\n"+randomString(10000)),
		newFile("jujud", tar.TypeReg, "jujud exe\n"+randomString(10000)),
	))
	content1.started = make(chan bool)

	files2 := []*file{
		newFile("replacement", tar.TypeReg, "replacement content"),
	}
	content2 := l.addContent("/y.tgz", makeArchive(files2...))

	dest1 := filepath.Join(s.dir, "dest1")
	s.downloader.Start(content1.url, dest1)
	<-content1.started

	// verify that the downloader really is in an intermediate state.
	m, err := filepath.Glob(filepath.Join(s.dir, "inprogress-*"))
	c.Assert(err, IsNil)
	c.Check(m, HasLen, 1)

	// Currently we can't interrupt the http get, so we
	// check to make sure that the Start is blocking while
	// the previous download completes.
	// When we fix this, this test will need to change.
	dest2 := filepath.Join(s.dir, "dest2")
	startDone := make(chan bool)
	go func() {
		s.downloader.Start(content2.url, dest2)
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
	c.Assert(status.URL, Equals, content2.url)
	c.Assert(status.Dir, Equals, dest2)
	c.Assert(status.Error, IsNil)
	assertDirContents(c, dest2, content2.url, files2)
}

// assertDirNames asserts that the given directory
// holds the given file or directory names.
func assertDirNames(c *C, dir string, names []string) {
	f, err := os.Open(dir)
	c.Assert(err, IsNil)
	defer f.Close()
	dnames, err := f.Readdirnames(0)
	c.Assert(err, IsNil)
	sort.Strings(dnames)
	sort.Strings(names)
	c.Assert(dnames, DeepEquals, names)
}

// assertDir Contents asserts that the given directory
// has the given file contents, and also contains the
// download-url.txt file recording the given url.
func assertDirContents(c *C, dir, url string, files []*file) {
	var wantNames []string
	for _, f := range files {
		wantNames = append(wantNames, f.header.Name)
	}
	wantNames = append(wantNames, "downloaded-url.txt")
	assertDirNames(c, dir, wantNames)
	assertFileContents(c, dir, "downloaded-url.txt", url)
	for _, f := range files {
		assertFileContents(c, dir, f.header.Name, f.contents)
	}
}

// assertFileContents asserts that the given file in the
// given directory has the given contents.
func assertFileContents(c *C, dir, file, contents string) {
	file = filepath.Join(dir, file)
	info, err := os.Stat(file)
	c.Assert(err, IsNil)
	c.Assert(info.Mode()&os.ModeType, Equals, os.FileMode(0))
	data, err := ioutil.ReadFile(file)
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, contents)
}

func makeArchive(files ...*file) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tarw := tar.NewWriter(gzw)

	for _, f := range files {
		err := tarw.WriteHeader(&f.header)
		if err != nil {
			panic(err)
		}
		_, err = tarw.Write([]byte(f.contents))
		if err != nil {
			panic(err)
		}
	}
	err := tarw.Close()
	if err != nil {
		panic(err)
	}
	err = gzw.Close()
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

type file struct {
	header   tar.Header
	contents string
}

func newFile(name string, ftype byte, contents string) *file {
	return &file{
		header: tar.Header{
			Typeflag:   ftype,
			Name:       name,
			Size:       int64(len(contents)),
			Mode:       0666,
			ModTime:    time.Now(),
			AccessTime: time.Now(),
			ChangeTime: time.Now(),
			Uname:      "ubuntu",
			Gname:      "ubuntu",
		},
		contents: contents,
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
func (srv *server) addContent(path string, data []byte) *content {
	c := &content{
		data: data,
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
