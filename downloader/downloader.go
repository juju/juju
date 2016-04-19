// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.downloader")

// Request holds a single download request.
type Request struct {
	// URL is the location from which the file will be downloaded.
	URL *url.URL

	// TargetDir is the directory into which the file will be downloaded.
	// It defaults to os.TempDir().
	TargetDir string
}

// Status represents the status of a completed download.
type Status struct {
	// File holds the downloaded data on success.
	File *os.File

	// Err describes any error encountered while downloading.
	Err error
}

// Download can download a file from the network.
type Download struct {
	tomb     tomb.Tomb
	done     chan Status
	openBlob func(*url.URL) (io.ReadCloser, error)
}

// New returns a new Download instance based on the provided request.
//
// openBlob is used to gain access to the blob, whether through an HTTP
// request or some other means.
func New(req Request, hostnameVerification utils.SSLHostnameVerification) *Download {
	d := &Download{
		done:     make(chan Status),
		openBlob: httpBlobOpener(hostnameVerification),
	}
	go d.run(req)
	return d
}

// Stop stops any download that's in progress.
func (d *Download) Stop() {
	d.tomb.Kill(nil)
	d.tomb.Wait()
}

// Done returns a channel that receives a status when the download has
// completed.  It is the receiver's responsibility to close and remove
// the received file.
func (d *Download) Done() <-chan Status {
	return d.done
}

// Wait blocks until the download completes or the abort channel receives.
func (dl *Download) Wait(abort <-chan struct{}) (Status, error) {
	defer dl.Stop()

	select {
	case <-abort:
		logger.Infof("download aborted")
		return Status{}, errors.New("aborted")
	case status := <-dl.Done():
		if status.Err != nil {
			return Status{}, errors.Trace(status.Err)
		}
		return status, nil
	}
}

func (d *Download) run(req Request) {
	defer d.tomb.Done()

	// TODO(dimitern) 2013-10-03 bug #1234715
	// Add a testing HTTPS storage to verify the
	// disableSSLHostnameVerification behavior here.
	file, err := download(req, d.openBlob)
	if err != nil {
		err = fmt.Errorf("cannot download %q: %v", req.URL, err)
	}

	status := Status{
		File: file,
		Err:  err,
	}
	select {
	case d.done <- status:
	case <-d.tomb.Dying():
		cleanTempFile(file)
	}
}

func httpBlobOpener(hostnameVerification utils.SSLHostnameVerification) func(*url.URL) (io.ReadCloser, error) {
	return func(url *url.URL) (io.ReadCloser, error) {
		// TODO(rog) make the download operation interruptible.
		client := utils.GetHTTPClient(hostnameVerification)
		resp, err := client.Get(url.String())
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("bad http response: %v", resp.Status)
		}
		return resp.Body, nil
	}
}

func download(req Request, openBlob func(*url.URL) (io.ReadCloser, error)) (file *os.File, err error) {
	dir := req.TargetDir
	if dir == "" {
		dir = os.TempDir()
	}
	tempFile, err := ioutil.TempFile(dir, "inprogress-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			cleanTempFile(tempFile)
		}
	}()

	reader, err := openBlob(req.URL)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	_, err = io.Copy(tempFile, reader)
	if err != nil {
		return nil, err
	}
	if _, err := tempFile.Seek(0, 0); err != nil {
		return nil, err
	}
	return tempFile, nil
}

func cleanTempFile(f *os.File) {
	if f != nil {
		f.Close()
		if err := os.Remove(f.Name()); err != nil {
			logger.Warningf("cannot remove temp file %q: %v", f.Name(), err)
		}
	}
}
