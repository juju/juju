package kvm

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	yaml "gopkg.in/yaml.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/environs/imagedownloads"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/utils/series"
)

const (
	// FType is the file type we want to fetch and use for kvm instances.
	FType = "disk1.img"
)

// Oner is an interface which allows us to use sync params to call
// imagedownloads.One or pass in a fake for testing.
type Oner interface {
	One() (*imagedownloads.Metadata, error)
}

// SyncParams conveys the information necessary for calling imagedownloads.One.
type SyncParams struct {
	arch, series, ftype string
	srcFunc             func() simplestreams.DataSource
}

// One implemnets Oner.
func (p SyncParams) One() (*imagedownloads.Metadata, error) {
	return imagedownloads.One(p.arch, p.series, p.ftype, p.srcFunc)
}

// Validate that our local type fulfull their implementations.
var _ Oner = (*SyncParams)(nil)
var _ Updater = (*updater)(nil)

// Updater is an interface to permit faking input in tests. The default
// implementation is updater, defined in this file.
type Updater interface {
	Update() error
}

// updater is our default Updater implementation.
type updater struct {
	md        *imagedownloads.Metadata
	fileCache *Image
	req       *http.Request
	client    *http.Client
	// gridFSCache imagestorage.Storage
}

// Update implements Updater. It checks the upstream copy of the image
// represented by the URL from simplestreams and fills the local cache if the
// local copy is missing or stale.
func (u *updater) Update() error {

	defer func() {
		err := u.fileCache.Close()
		if err != nil {
			logger.Errorf("failed defer: %s", err.Error())
		}
	}()
	// TODO(ro) 2016-11-15 once we support gridFS cache we'll need to compare
	// the ModTime on that to our local cache and upstream. Then we would
	// fill the cache(s) appropriately; i.e. local from controller or both from
	// upstream.
	u.req.Header.Set("If-Modified-Since", u.fileCache.ModTime)
	if u.client == nil {
		u.client = &http.Client{}
		// TODO(ro) 2016-11-15 When we start using 1.8 in Juju, remove this
		// check redirect function. This is here for two reasons. The first is
		// that simplestreams url paths point at a URL that always redirects to
		// the actual file. I don't quite understand why we look up a location
		// to get redirected to yet another location. The second reason is that
		// Go does not copy headers from the original request over to redirect
		// loactions that it follows, cf.
		// https://github.com/golang/go/issues/4800 . This has apparently been
		// closed, but isn't making it in until the 1.8 release due out early
		// 2017.
		u.client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
			if len(via) > 9 {
				return errors.Errorf("too many redirects")
			}
			if len(via) == 0 {
				return nil
			}
			r.Header.Set("If-Modified-Since", u.fileCache.ModTime)
			return nil
		}
	}

	resp, err := u.client.Do(u.req)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			logger.Errorf("failed defer: %s", err.Error())
		}
	}()

	switch resp.StatusCode {
	case 200, 301, 302, 303, 307:
		return u.fillCache(resp)
	case 304:
		return nil
	default:
		return errors.Errorf("got %q fetching kvm image URL %q",
			resp.Status,
			u.req.URL.String())
	}
}

// fillCache populates the image and metadata files.
func (u *updater) fillCache(r *http.Response) error {
	defer func() {
		err := r.Body.Close()
		if err != nil {
			logger.Errorf("failed defer: %s", err.Error())
		}
	}()
	// Sanity check the response data size. Not sure if we should error out
	// here. Is it possible simplestreams might have a bug or be out of
	// date/sync?
	if r.ContentLength != u.md.Size {
		logger.Infof("got content-length %q, expected %q", r.ContentLength, u.md.Size)
	}
	// TODO(ro) 2016-11-15 Once we support gridFS caching we'll need to use
	// io.Teereader to update both local and gridFS copies from the same
	// stream.
	return u.fileCache.write(r.Body, u.md)
}

func newDefaultUpdater(md *imagedownloads.Metadata, pathfinder func(string) (string, error)) (*updater, error) {
	// TODO(ro) 2016-11-14 We need to pass the image storage service down
	// through the call to the factory and then to the 'container' on through
	// to the Sync call in order to populate a mongo cache, cf.
	// state.ImageStorage()
	// gridFSCache := func () imagestorage.Storage
	i, err := maybeNewImage(md, pathfinder)
	if err != nil {
		return nil, errors.Trace(err)
	}

	dlURL, err := md.DownloadURL()
	if err != nil {
		return nil, errors.Trace(err)
	}
	req, err := http.NewRequest("GET", dlURL.String(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &updater{md: md, fileCache: i, req: req}, nil
}

// Sync updates the local cached images by reading the simplestreams
// data and updating if necessary. It retrieves metadata
// information from Oner and updates local cache(s) via Updater.
func Sync(o Oner, u Updater) error {
	md, err := o.One()
	if err != nil {
		return err
	}
	if u == nil {
		u, err = newDefaultUpdater(md, paths.DataDir)
		if err != nil {
			return errors.Trace(err)
		}
	}
	err = u.Update()
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Image represents a server image. In the KVM case a disk1.img, but could be
// type: e.g. tar.gz.
type Image struct {
	Arch         string `yaml:"arch,omitempty"`
	FilePath     string `yaml:"file_path,omitempty"`
	MetadataPath string `yaml:"md_path,omitempty"`
	FType        string `yaml:"ftype,omitempty"`
	Series       string `yaml:"series,omitempty"`
	ModTime      string `yaml:"mtime,omitempty"`
	SHA256       string `yaml:"sha256,omitempty"`
	Size         int64  `yaml:"size,omitempty"`
	fileHandle   *os.File
	mdHandle     *os.File
}

// Type returns the type of container this image is for. This is required by
// for imagestorage.Metadata in the imagestorage.Storage.AddImage method call.
func (i *Image) Type() string {
	return "kmv"
}

// Close closes the file handles embedded in Image.
func (i *Image) Close() error {
	err := i.mdHandle.Close()
	if err != nil {
		err = errors.Trace(err)
	}
	err2 := i.fileHandle.Close()
	if err2 != nil {
		err = errors.Trace(err2)
	}
	return err
}

// write saves the stream to disk and updates the metadata file.
func (i *Image) write(r io.Reader, md *imagedownloads.Metadata) error {
	// First empty the existing file.
	err := i.fileHandle.Truncate(0)
	if err != nil {
		return errors.Errorf("got %s when truncating %s",
			err.Error(),
			i.FilePath)
	}
	wrote, err := io.Copy(i.fileHandle, r)
	if err != nil {
		defer i.cleanup()
		return errors.Trace(err)
	}
	if wrote != md.Size {
		// This should likely be an error, with cleanup called prior to
		// returning.
		logger.Errorf("wrote %d, but expected to write %d", wrote, md.Size)
	}
	// We set ModTime to the time we got the update, now.
	i.ModTime = time.Now().Format(http.TimeFormat)
	// Size and hash are the only members that need updating.
	i.Size = md.Size
	i.SHA256 = md.SHA256
	// TODO(ro) 2016-11-15 validate size and hash.
	b, err := yaml.Marshal(i)
	if err != nil {
		defer i.cleanup()
		return errors.Trace(err)
	}
	err = i.mdHandle.Truncate(0)
	_, err = i.mdHandle.Write(b)
	if err != nil {
		defer i.cleanup()
		return errors.Trace(err)
	}
	return nil
}

// cleanup attempts to remove the imagefile and metadata. It can be called if
// things don't work out. E.g. sha256 mismatch, incorrect size...
func (i *Image) cleanup() {
	err := os.Remove(i.FilePath)
	if err != nil {
		logger.Errorf("got error %q removing %q", err.Error(), i.FilePath)
	}
	err = os.Remove(i.MetadataPath)
	if err != nil {
		logger.Errorf("got %q removing %q", err.Error(), i.MetadataPath)
	}

}

// New returns a new Image from a simplestreams derived url and a pathfinder
// func. pathfinder is the signature provided by juju.paths.DataDir. If the
// image is already cached the returned image is populated with the cached data. If
// not, it populates it with the metadata passed in.
func maybeNewImage(md *imagedownloads.Metadata, pathfinder func(string) (string, error)) (*Image, error) {

	// Setup names and paths.
	dlURL, err := md.DownloadURL()
	if err != nil {
		return nil, errors.Trace(err)
	}
	fname := path.Base(dlURL.String())
	baseDir, err := pathfinder(series.HostSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = maybeMakeDirs(baseDir); err != nil {
		return nil, errors.Trace(err)
	}
	fpath := baseDir + "/kvm/images/" + fname
	mdPath := baseDir + "/kvm/metadata/" + fname

	// Get handles on the necessary files.
	mdh, err := os.OpenFile(mdPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, errors.Trace(err)
	}
	fh, err := os.OpenFile(fpath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If there's metadata load it and return the img with handles.
	var img *Image
	mdInfo, err := mdh.Stat()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if mdInfo.Size() > 0 {
		img = &Image{mdHandle: mdh, fileHandle: fh}
		data, err := ioutil.ReadAll(mdh)
		if err != nil {
			defer imgCloser(img)
			return nil, errors.Trace(err)
		}
		_, _ = img.mdHandle.Seek(0, 0)
		err = yaml.Unmarshal(data, img)
		if err != nil {
			defer imgCloser(img)
			return nil, errors.Trace(err)
		}
		return img, nil

	}

	// Otherwise it is new and we return an empty image with handles and
	// metadata from imagedownloads.
	return &Image{
		Arch:         md.Arch,
		FType:        md.FType,
		FilePath:     fpath,
		MetadataPath: mdPath,
		ModTime:      time.Time{}.Format(http.TimeFormat),
		SHA256:       md.SHA256,
		Series:       md.Release,
		Size:         md.Size,
		mdHandle:     mdh,
		fileHandle:   fh,
	}, nil
}

func imgCloser(img *Image) {
	err := img.Close()
	if err != nil {
		logger.Errorf("failed defer: %s", err.Error())
	}
}

// maybeMkdirs creates the kvm image directories if they don't exist.
func maybeMakeDirs(base string) error {
	for _, path := range []string{"kvm/images", "kvm/metadata"} {
		if err := os.MkdirAll(filepath.Join(base, path), 0700); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
