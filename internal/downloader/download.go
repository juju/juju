// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package downloader

import (
	"context"
	"io"
	"net/url"
	"os"

	"github.com/juju/errors"
)

// Request holds a single download request.
type Request struct {
	// ArchiveSha256 is the string containing the charm archive sha256 hash.
	ArchiveSha256 string

	// URL is the location from which the file will be downloaded.
	URL *url.URL

	// TargetDir is the directory into which the file will be downloaded.
	// It defaults to os.TempDir().
	TargetDir string

	// Verify is used to ensure that the download result is correct. If
	// the download is invalid then the func must return errors.NotValid.
	// If no func is provided then no verification happens.
	Verify func(*os.File) error
}

// Status represents the status of a completed download.
type Status struct {
	// Filename is the name of the file which holds the downloaded
	// data on success.
	Filename string

	// Err describes any error encountered while downloading.
	Err error
}

// StartDownload starts a new download as specified by `req` using
// `openBlob` to actually pull the remote data.
func StartDownload(ctx context.Context, req Request, openBlob func(Request) (io.ReadCloser, error)) *Download {
	if openBlob == nil {
		openBlob = NewHTTPBlobOpener(false)
	}
	dl := &Download{
		done:     make(chan Status, 1),
		openBlob: openBlob,
	}
	go dl.run(ctx, req)
	return dl
}

// Download can download a file from the network.
type Download struct {
	done     chan Status
	openBlob func(Request) (io.ReadCloser, error)
}

// Done returns a channel that receives a status when the download has
// completed or is aborted. Exactly one Status value will be sent for
// each download once it finishes (successfully or otherwise) or is
// aborted.
//
// It is the receiver's responsibility to handle and remove the
// downloaded file.
func (dl *Download) Done() <-chan Status {
	return dl.done
}

// Wait blocks until the download finishes (successfully or
// otherwise), or the download is aborted. There will only be a
// filename if err is nil.
func (dl *Download) Wait() (string, error) {
	// No select required here because each download will always
	// return a value once it completes. Downloads can be aborted via
	// the Abort channel provided a creation time.
	status := <-dl.Done()
	return status.Filename, errors.Trace(status.Err)
}

func (dl *Download) run(ctx context.Context, req Request) {
	// TODO(dimitern) 2013-10-03 bug #1234715
	// Add a testing HTTPS storage to verify the
	// disableSSLHostnameVerification behavior here.
	filename, err := dl.download(ctx, req)
	if err != nil {
		err = errors.Trace(err)
	} else {
		logger.Infof(ctx, "download complete (%q)", req.URL)
		err = verifyDownload(ctx, filename, req)
		if err != nil {
			_ = os.Remove(filename)
			filename = ""
		}
	}

	// No select needed here because the channel has a size of 1 and
	// will only be written to once.
	dl.done <- Status{
		Filename: filename,
		Err:      err,
	}
}

func (dl *Download) download(ctx context.Context, req Request) (filename string, err error) {
	logger.Infof(ctx, "downloading from %s", req.URL)

	dir := req.TargetDir
	if dir == "" {
		dir = os.TempDir()
	}
	tempFile, err := os.CreateTemp(dir, "inprogress-")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() {
		_ = tempFile.Close()
		if err != nil {
			_ = os.Remove(tempFile.Name())
		}
	}()

	blobReader, err := dl.openBlob(req)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = blobReader.Close() }()

	reader := &abortableReader{r: blobReader, abort: ctx.Done()}
	_, err = io.Copy(tempFile, reader)
	if err != nil {
		return "", errors.Trace(err)
	}

	return tempFile.Name(), nil
}

// abortableReader wraps a Reader, returning an error from Read calls
// if the abort channel provided is closed.
type abortableReader struct {
	r     io.Reader
	abort <-chan struct{}
}

// Read implements io.Reader.
func (ar *abortableReader) Read(p []byte) (int, error) {
	select {
	case <-ar.abort:
		return 0, errors.New("download aborted")
	default:
	}
	return ar.r.Read(p)
}

func verifyDownload(ctx context.Context, filename string, req Request) error {
	if req.Verify == nil {
		return nil
	}

	file, err := os.Open(filename)
	if err != nil {
		return errors.Annotate(err, "opening for verify")
	}
	defer func() { _ = file.Close() }()

	if err := req.Verify(file); err != nil {
		return errors.Trace(err)
	}
	logger.Infof(ctx, "download verified (%q)", req.URL)
	return nil
}
