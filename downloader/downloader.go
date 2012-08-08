package downloader

import (
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"net/http"
	"os"
)

// TempDir is the temporary directory where downloaded files
// are stored. If it is empty, it uses the default directory for temporary
// files (see os.TempDir).
var TempDir string

type status struct {
	// Err describes any error encountered while downloading.
	Err error
	// File holds the file that the URL has downloaded to.
	File *os.File
}

// Downloader can download a file from the network.
type Downloader struct {
	tomb tomb.Tomb
	done chan Status
	url  string
	w    io.Writer
}

// New returns a new Downloader downloading the given
// url into a temporary file.
func New(url string) *Downloader {
	return &Downloader{
		done: make(chan Status),
	}
}

// Stop stops any download that's in progress and returns
// any error encountered
func (d *Downloader) Stop() error {
	d.tomb.Kill(nil)
	return d.tomb.Wait()
}

// Done returns a channel that receives a value when
// a file has been successfully downloaded and installed.
func (d *Downloader) Done() <-chan Status {
	return d.done
}

// downloadOne runs a single download attempt.
type downloadOne struct {
	tomb tomb.Tomb
	done chan Status
	url  string
	w    io.Writer
}

func (d *downloadOne) stop() error {
	d.tomb.Kill(nil)
	return d.tomb.Wait()
}

func (d *downloadOne) run() {
	defer d.tomb.Done()
	file, err := d.download()
	if err != nil {
		err = fmt.Errorf("cannot download %q: %v", d.url, err)
	}
	if !d.sendStatus(Status{
		URL:  d.url,
		File: file,
		Err:  err,
	}) {
		// If we have failed to send the status then we need
		// to clean up the temporary file ourselves.
		if file != nil {
			cleanTempFile(file)
		}
	}
}

func cleanTempFile(f *os.File) {
	f.Close()
	if err := os.Remove(f.Name()); err != nil {
		log.Printf("downloader: cannot remove temporary file: %v", err)
	}
}

func (d *downloadOne) sendStatus(status Status) bool {
	// If we have been interrupted while downloading
	// then don't try to send the status.
	// This is to make tests easier - when we can interrupt
	// downloads, this can go away.
	select {
	case <-d.tomb.Dying():
		return false
	default:
	}
	select {
	case d.done <- status:
	case <-d.tomb.Dying():
		return false
	}
	return true
}

func (d *downloadOne) download() (file *os.File, err error) {
	tmpFile, err := ioutil.TempFile(TempDir, "juju-download-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			cleanTempFile(tmpFile)
		}
	}()
	// TODO(rog) make the Get operation interruptible.
	resp, err := http.Get(d.url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad http response %v", resp.Status)
	}
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return nil, err
	}
	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		return nil, err
	}
	return tmpFile, nil
}
