package downloader_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/downloader"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
	"log"
)

type suite struct{}

var _ = Suite(suite{})

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

func (suite) TestDownloader(c *C) {
	l := newServer()
	files := []*file{
		newFile("bar", tar.TypeReg, "bar contents"),
		newFile("foo", tar.TypeReg, "foo contents"),
	}

	content := l.addContent("/archive.tgz", makeArchive(files...))
	content.started = nil
	dl := downloader.New()
	dir := c.MkDir()
	dest := filepath.Join(dir, "dest")
	dl.Start(content.url, dest)
	status := <-dl.Done()
	c.Assert(status.Error, IsNil)
	c.Assert(status.Dir, Equals, dest)
	c.Assert(status.URL, Equals, content.url)
	assertDirectoryContents(c, dest, content.url, files)
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

// assertDirectoryContents asserts that the given directory
// has the given file contents, and also contains the
// download-url.txt file recording the given url.
func assertDirectoryContents(c *C, dir, url string, files []*file) {
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

func (l *server) addContent(name string, data []byte) *content {
	c := &content{
		data:    data,
		started: make(chan bool),
	}
	c.url = fmt.Sprintf("http://%v%s", l.l.Addr(), name)
	l.contents[name] = c
	return c
}

func (l *server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	log.Printf("got http req %#v", req)
	log.Printf("path %q", req.URL.Path)
	defer log.Printf("done http req")
	c := l.contents[req.URL.Path]
	if c == nil {
		http.NotFound(w, req)
		return
	}
	w.Write(c.data)
//[0 : len(c.data)-1])
//	if c.started != nil {
//		c.started <- true
//	}
//	w.Write(c.data[len(c.data)-1:])
}
