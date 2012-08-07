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

// Status represents the status of a completed download.
// It is the receiver's responsibility to close and remove
// the file.
type Status struct {
	// URL holds the downloaded URL.
	URL string
	// Err describes any error encountered while downloading.
	Err error
	// File holds the file that the URL has downloaded to.
	File *os.File
}

// Downloader can download a file from the network.
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

// Start requests that the given URL be downloaded and written
// to the given Writer.
//
// If Start is called while another download is already in progress, the
// previous download will be cancelled.
func (d *Downloader) Start(url string) {
	if d.current != nil {
		d.Stop()
	}
	d.current = &downloadOne{
		url:  url,
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
// a file has been successfully downloaded and installed.
func (d *Downloader) Done() <-chan Status {
	return d.done
}

// downloadOne runs a single download attempt.
type downloadOne struct {
	tomb tomb.Tomb
	done chan Status
	url  string
	w io.Writer
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
	status := Status{
		URL: d.url,
		File: file,
		Err: err,
	}
	// If we have been interrupted while downloading
	// then don't try to send the status.
	// This is to make tests easier - when we can interrupt
	// downloads, this can go away.
	select {
	case <-d.tomb.Dying():
		return
	default:
	}
	select {
	case d.done <- status:
	case <-d.tomb.Dying():
	}
}

func (d *downloadOne) download() (file *os.File, err error) {
	tmpFile, err := ioutil.TempFile("", "juju-download-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			tmpFile.Close()
			if err := os.Remove(tmpFile.Name()); err != nil {
				log.Printf("downloader: cannot remove temporary file: %v", err)
			}
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
