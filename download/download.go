package download
import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Unarchive creates the given directory, downloads a gzipped
// tar archive from the given URL and unarchives it into the
// directory. It also records the URL there by writing it to a file called
// downloaded-url.txt.
//
// Unarchive returns an error if the directory already exists
// or its parent cannot be written. The archive must contain
// only regular files in its top level directory.
func Unarchive(url, dir string) (err error) {
	parent, _ := filepath.Split(dir)
	tmpdir, err := ioutil.TempDir(parent, "download-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tmpdir)
		}
	}()
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	r, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer r.Close()

	tr := tar.NewReader(r)
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
		name := filepath.Join(tmpdir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return fmt.Errorf("tar extract %q failed: %v", name, err)
		}
	}
	if err := ioutil.WriteFile(filepath.Join(tmpdir, "downloaded-url.txt"), []byte(url), 0666); err != nil {
		return err
	}

	return os.Rename(tmpdir, dir)
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

// DownloadStatus represents the status of a completed download.
type DownloadStatus struct {
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
	done chan DownloadStatus
}

// NewDownloader returns a new Downloader instance.
// Nothing will be downloaded until Start is called.
func NewDownloader() *Downloader {
	return &Downloader{
		done: make(chan DownloadStatus),
	}
}

// Start requests that the given tools be downloaded
// into the given directory. If the directory already exists,
// nothing is done and the download is counted as successful.
// If Start is called while another download is already in progress,
// the previous download will be cancelled.
func (d *Downloader) Start(url, dir string) {

	if d.current != nil {
		// If we are already downloading the right tools,
		// we need do nothing.
		if *d.current.tools == *tools {
			return
		}
		if err := d.current.stop(); err != nil {
			log.Printf("downloader: error stopping downloader: %v", err)
		}
		d.current = nil
	}
	d.current = &downloadOne{done: done}
	go d.current.run(url, dir)
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
func (d *Downloader) Done() <-chan DownloadStatus {
	return d.done
}

type downloadOne struct {
	tomb tomb.Tomb
	done chan DownloadStatus
}

func (d *downloadOne) stop() error {
	d.tomb.Kill(nil)
	return d.tomb.Wait()
}

func (d *downloadOne) run(url, dir string) {
	defer d.tomb.Done()
	if info, err := os.Stat(dir); err == nil || !os.IsNotFound(err) {
	// TODO (rog) make Unarchive interruptible.
	status := DownloadStatus{
		URL: url,
		Dir: dir,
		Error: Unarchive(url, dir),
	}
	select {
	case d.done <- status:
	case <-d.tomb.Dying():
	}
}
