// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kvm

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/os/series"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/environs/simplestreams"
)

// BIOSFType is the file type we want to fetch and use for kvm instances which
// boot using a legacy BIOS boot loader.
const BIOSFType = "disk1.img"

// UEFIFType is the file type we want to fetch and use for kvm instances which
// boot using UEFI. In our case this is ARM64.
const UEFIFType = "uefi1.img"

// Oner gets the one matching item from simplestreams.
type Oner interface {
	One() (*imagedownloads.Metadata, error)
}

// syncParams conveys the information necessary for calling imagedownloads.One.
type syncParams struct {
	arch, series, stream, fType string
	srcFunc                     func() simplestreams.DataSource
}

// One implements Oner.
func (p syncParams) One() (*imagedownloads.Metadata, error) {
	if err := p.exists(); err != nil {
		return nil, errors.Trace(err)
	}
	return imagedownloads.One(p.arch, p.series, p.stream, p.fType, p.srcFunc)
}

func (p syncParams) exists() error {
	fName := backingFileName(p.series, p.arch)
	baseDir, err := paths.DataDir(series.MustHostSeries())
	if err != nil {
		return errors.Trace(err)
	}
	imagePath := filepath.Join(baseDir, kvm, guestDir, fName)

	if _, err := os.Stat(imagePath); err == nil {
		return errors.AlreadyExistsf("%q %q image for exists at %q", p.series, p.arch, imagePath)
	}
	return nil
}

// Validate that our types fulfill their implementations.
var _ Oner = (*syncParams)(nil)
var _ Fetcher = (*fetcher)(nil)

// Fetcher is an interface to permit faking input in tests. The default
// implementation is updater, defined in this file.
type Fetcher interface {
	Fetch() error
	Close()
}

type fetcher struct {
	metadata         *imagedownloads.Metadata
	req              *http.Request
	client           *http.Client
	image            *Image
	imageDownloadURL string
}

// Fetch implements Fetcher. It fetches the image file from simplestreams and
// delegates writing it out and creating the qcow3 backing file to Image.write.
func (f *fetcher) Fetch() error {
	resp, err := f.client.Do(f.req)
	if err != nil {
		return errors.Trace(err)
	}

	defer func() {
		err = resp.Body.Close()
		if err != nil {
			logger.Debugf("failed defer %q", errors.Trace(err))
		}
	}()

	if resp.StatusCode != 200 {
		f.image.cleanup()
		return errors.NotFoundf(
			"got %d fetching image %q", resp.StatusCode, path.Base(
				f.req.URL.String()))
	}
	err = f.image.write(resp.Body, f.metadata, f.imageDownloadURL)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Close calls images cleanup method for deferred closing of the image tmpFile.
func (f *fetcher) Close() {
	f.image.cleanup()
}

type ProgressCallback func(message string)

// Sync updates the local cached images by reading the simplestreams data and
// caching if an image matching the constraints doesn't exist. It retrieves
// metadata information from Oner and updates local cache via Fetcher.
// A ProgressCallback can optionally be passed which will get update messages
// as data is copied.
func Sync(o Oner, f Fetcher, imageDownloadURL string, progress ProgressCallback) error {
	md, err := o.One()
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// We've already got a backing file for this series/architecture.
			return nil
		}
		return errors.Trace(err)
	}
	if f == nil {
		f, err = newDefaultFetcher(md, imageDownloadURL, paths.DataDir, progress)
		if err != nil {
			return errors.Trace(err)
		}
		defer f.Close()
	}
	err = f.Fetch()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Image represents a server image.
type Image struct {
	FilePath string
	progress ProgressCallback
	tmpFile  *os.File
	runCmd   runFunc
}

type progressWriter struct {
	callback    ProgressCallback
	url         string
	total       uint64
	maxBytes    uint64
	startTime   *time.Time
	lastPercent int
	clock       clock.Clock
}

var _ io.Writer = (*progressWriter)(nil)

func (p *progressWriter) Write(content []byte) (n int, err error) {
	if p.clock == nil {
		p.clock = clock.WallClock
	}
	p.total += uint64(len(content))
	if p.startTime == nil {
		now := p.clock.Now()
		p.startTime = &now
		return len(content), nil
	}
	if p.callback != nil {
		elapsed := p.clock.Now().Sub(*p.startTime)
		// Avoid measurements that aren't interesting
		if elapsed > time.Millisecond {
			percent := (float64(p.total) * 100.0) / float64(p.maxBytes)
			intPercent := int(percent + 0.5)
			if p.lastPercent != intPercent {
				bps := uint64((float64(p.total) / elapsed.Seconds()) + 0.5)
				p.callback(fmt.Sprintf("copying %s %d%% (%s/s)", p.url, intPercent, humanize.Bytes(bps)))
				p.lastPercent = intPercent
			}
		}
	}
	return len(content), nil
}

// write saves the stream to disk and updates the metadata file.
func (i *Image) write(r io.Reader, md *imagedownloads.Metadata, imageDownloadURL string) error {
	tmpPath := i.tmpFile.Name()
	defer func() {
		err := i.tmpFile.Close()
		if err != nil {
			logger.Errorf("failed to close %q %s", tmpPath, err)
		}
		err = os.Remove(tmpPath)
		if err != nil {
			logger.Errorf("failed to remove %q after use %s", tmpPath, err)
		}

	}()

	hash := sha256.New()
	var writer io.Writer
	if i.progress == nil {
		writer = io.MultiWriter(i.tmpFile, hash)
	} else {
		dlURL, _ := md.DownloadURL(imageDownloadURL)
		progWriter := &progressWriter{
			url:      dlURL.String(),
			callback: i.progress,
			maxBytes: uint64(md.Size),
			total:    0,
		}
		writer = io.MultiWriter(i.tmpFile, hash, progWriter)
	}
	_, err := io.Copy(writer, r)
	if err != nil {
		i.cleanup()
		return errors.Trace(err)
	}

	result := fmt.Sprintf("%x", hash.Sum(nil))
	if result != md.SHA256 {
		i.cleanup()
		return errors.Errorf(
			"hash sum mismatch for %s: %s != %s", i.tmpFile.Name(), result, md.SHA256)
	}

	// TODO(jam): 2017-03-19 If this is slow, maybe we want to add a progress step for it, rather than only
	// indicating download progress.
	output, err := i.runCmd(
		"", "qemu-img", "convert", "-f", "qcow2", tmpPath, i.FilePath)
	logger.Debugf("qemu-image convert output: %s", output)
	if err != nil {
		i.cleanupAll()
		return errors.Trace(err)
	}
	return nil
}

// cleanup attempts to close and remove the tempfile download image. It can be
// called if things don't work out. E.g. sha256 mismatch, incorrect size...
func (i *Image) cleanup() {
	if err := i.tmpFile.Close(); err != nil {
		logger.Debugf("%s", err.Error())
	}

	if err := os.Remove(i.tmpFile.Name()); err != nil {
		logger.Debugf("got %q removing %q", err.Error(), i.tmpFile.Name())
	}
}

// cleanupAll cleans up the possible backing file as well.
func (i *Image) cleanupAll() {
	i.cleanup()
	err := os.Remove(i.FilePath)
	if err != nil {
		logger.Debugf("got %q removing %q", err.Error(), i.FilePath)
	}
}

func newDefaultFetcher(md *imagedownloads.Metadata, imageDownloadURL string, pathfinder func(string) (string, error), callback ProgressCallback) (*fetcher, error) {
	i, err := newImage(md, imageDownloadURL, pathfinder, callback)
	if err != nil {
		return nil, errors.Trace(err)
	}
	dlURL, err := md.DownloadURL(imageDownloadURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req, err := http.NewRequest("GET", dlURL.String(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := &http.Client{}
	return &fetcher{metadata: md, image: i, client: client, req: req, imageDownloadURL: imageDownloadURL}, nil
}

func newImage(md *imagedownloads.Metadata, imageDownloadURL string, pathfinder func(string) (string, error), callback ProgressCallback) (*Image, error) {
	// Setup names and paths.
	dlURL, err := md.DownloadURL(imageDownloadURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	baseDir, err := pathfinder(series.MustHostSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Closing this is deferred in Image.write.
	fh, err := ioutil.TempFile("", fmt.Sprintf("juju-kvm-%s-", path.Base(dlURL.String())))
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &Image{
		FilePath: filepath.Join(
			baseDir, kvm, guestDir, backingFileName(md.Release, md.Arch)),
		tmpFile:  fh,
		runCmd:   run,
		progress: callback,
	}, nil
}

func backingFileName(series, arch string) string {
	// TODO(ro) validate series and arch to be sure they are in the right order.
	return fmt.Sprintf("%s-%s-backing-file.qcow", series, arch)
}
