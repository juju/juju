package downloader

import (
	"bytes"
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/tomb"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"log"
)

// Status represents the status of a completed download.
type Status struct {
	// URL holds the downloaded URL.
	URL string
	// Dir holds the directory that it has been downloaded to.
	Dir string
	// Error describes any error encountered downloading the tools.
	Error error
}

// Downloader can download an archived directory from the network.
type Downloader struct {
	current *downloadOne
	done    chan Status
}

// New returns a new Downloader instance.
// Nothing will be downloaded until Start is called.
func New() *Downloader {
	return &Downloader{
		done: make(chan Status),
	}
}

// Start requests that the given tools be downloaded into the given
// directory.  If the directory already exists, nothing is done and the
// download is counted as successful.  The url must contain a
// gzipped tar archive holding single-level directory containing regular files
// only. The URL is recorded by writing it to a file in the destination directory
// called "downloaded-url.txt".
//
// If Start is called while another download is already in progress, the
// previous download will be cancelled.
func (d *Downloader) Start(url, dir string) {
	log.Printf("starting %q -> %q", url, dir)
	if d.current != nil {
		// We're already downloading the same thing, so do nothing.
		if url == d.current.url && dir == d.current.dir {
			return
		}
		d.Stop()
	}
	d.current = &downloadOne{
		url:  url,
		dir:  dir,
		done: d.done,
	}
	go d.current.run()
}

// Stop stops any download that's in progress.
func (d *Downloader) Stop() {
	if d.current != nil {
		d.current.stop()
		d.current = nil
	}
}

// Done returns a channel that receives a value when
// some tools have been successfully downloaded and installed.
func (d *Downloader) Done() <-chan Status {
	return d.done
}

type downloadOne struct {
	tomb tomb.Tomb
	done chan Status
	url  string
	dir  string
}

func (d *downloadOne) stop() error {
	d.tomb.Kill(nil)
	return d.tomb.Wait()
}

func (d *downloadOne) run() {
	defer d.tomb.Done()
	err := d.download()
	if err != nil {
		err = fmt.Errorf("cannot download %q to %q: %v", d.url, d.dir, err)
	}
	status := Status{
		URL:   d.url,
		Dir:   d.dir,
		Error: err,
	}
	log.Printf("about to send status on %p: %v", d.done, status)
	select {
	case d.done <- status:
		log.Printf("sent status")
	case <-d.tomb.Dying():
		log.Printf("downloadOne stopped")
	}
}

func (d *downloadOne) download() (err error) {
	log.Printf("download start")
	defer log.Printf("download done")
	// If the directory already exists, we assume that the
	// download has already taken place and succeeded.
	if info, err := os.Stat(d.dir); err == nil && info.IsDir() {
		return nil
	}
	resp, err := http.Get(d.url)
	if err != nil {
		return err
	}
	log.Printf("got http response")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("bad http response %v", resp.Status)
	}
	parent, _ := filepath.Split(d.dir)
	tmpdir, err := ioutil.TempDir(parent, "inprogress-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpdir)
		}
	}()
	log.Printf("unarchiving")
	// TODO(rog) make the unarchive operation interruptible.
	err = d.unarchive(resp.Body, tmpdir)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(filepath.Join(tmpdir, "downloaded-url.txt"), []byte(d.url), 0666); err != nil {
		return err
	}
	return os.Rename(tmpdir, d.dir)
}

// unarchive unarchives the gzipped tar archive from the given reader
// into the given directory.  The archive must contain only regular
// files in its top level directory.
func (d *downloadOne) unarchive(r io.Reader, dir string) (err error) {
var buf bytes.Buffer
io.Copy(&buf, r)
ioutil.WriteFile("/tmp/foo2.gz", buf.Bytes(), 0666)
	zr, err := gzip.NewReader(&buf)
	if err != nil {
		return err
	}
	defer zr.Close()

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		// TODO (rog) relax the "no sub-directories" restriction.
		if strings.Contains(hdr.Name, "/\\") {
			return fmt.Errorf("bad name %q in tools archive", hdr.Name)
		}
		name := filepath.Join(dir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return fmt.Errorf("tar extract %q failed: %v", name, err)
		}
	}
	return nil
}

func writeFile(name string, mode os.FileMode, r io.Reader) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
