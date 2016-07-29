// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path"
	"strings"

	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/internal/blobstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

// Blob represents a blob of data from the charm store.
type Blob struct {
	blobstore.ReadSeekCloser

	// Size holds the total size of the blob.
	Size int64

	// Hash holds the hash checksum of the blob.
	Hash string
}

var preV5ArchiveFields = []string{
	"size",
	"blobhash",
	"blobname",
	"prev5blobhash",
	"prev5blobsize",
}

// OpenBlob returns the blob associated with the given URL.
func (s *Store) OpenBlob(id *router.ResolvedURL) (*Blob, error) {
	return s.openBlob(id, false)
}

// OpenBlob returns the blob associated with the given URL.
// As required by pre-v5 versions of the API, it will return a blob
// with a hacked-up metadata.yaml that elides the Series field.
func (s *Store) OpenBlobPreV5(id *router.ResolvedURL) (*Blob, error) {
	return s.openBlob(id, true)
}

func (s *Store) openBlob(id *router.ResolvedURL, preV5 bool) (*Blob, error) {
	entity, err := s.FindEntity(id, FieldSelector(preV5ArchiveFields...))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	r, size, err := s.BlobStore.Open(entity.BlobName)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open archive data for %s", id)
	}
	hash := entity.BlobHash

	if entity.PreV5BlobHash != entity.BlobHash && preV5 {
		// The v5 blob is different so we open the blob suffix that
		// contains the metadata hack.
		r2, size2, err := s.BlobStore.Open(preV5CompatibilityBlobName(entity.BlobName))
		if err != nil {
			r.Close()
			return nil, errgo.Notef(err, "cannot find pre-v5 hack blob")
		}
		r = newMultiReadSeekCloser(r, r2)
		size += size2
		hash = entity.PreV5BlobHash
	}
	return &Blob{
		ReadSeekCloser: r,
		Size:           size,
		Hash:           hash,
	}, nil
}

type multiReadSeekCloser struct {
	readers []blobstore.ReadSeekCloser
	io.ReadSeeker
}

func newMultiReadSeekCloser(readers ...blobstore.ReadSeekCloser) blobstore.ReadSeekCloser {
	br := make([]io.ReadSeeker, len(readers))
	for i, r := range readers {
		br[i] = r
	}
	return &multiReadSeekCloser{
		readers:    readers,
		ReadSeeker: utils.NewMultiReaderSeeker(br...),
	}
}

func (r *multiReadSeekCloser) Close() error {
	for _, r := range r.readers {
		r.Close()
	}
	return nil
}

// OpenBlobFile opens the file with the given path from the
// given blob and returns a reader for its contents,
// and its size.
//
// If no such file was found, it returns an error
// with a params.ErrNotFound cause.
//
// If the file is actually a directory in the blob, it returns
// an error with a params.ErrForbidden cause.
func (s *Store) OpenBlobFile(blob *Blob, filePath string) (io.ReadCloser, int64, error) {
	zipReader, err := zip.NewReader(ReaderAtSeeker(blob), blob.Size)
	if err != nil {
		return nil, 0, errgo.Notef(err, "cannot read archive data")
	}

	filePath = strings.TrimPrefix(path.Clean(filePath), "/")
	for _, file := range zipReader.File {
		if path.Clean(file.Name) != filePath {
			continue
		}
		// The file is found.
		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			return nil, 0, errgo.WithCausef(nil, params.ErrForbidden, "directory listing not allowed")
		}
		content, err := file.Open()
		if err != nil {
			return nil, 0, errgo.Notef(err, "unable to read file %q", filePath)
		}
		return content, fileInfo.Size(), nil
	}
	return nil, 0, errgo.WithCausef(nil, params.ErrNotFound, "file %q not found in the archive", filePath)
}

// OpenCachedBlobFile opens a file from the given entity's archive blob.
// The file is identified by the provided fileId. If the file has not
// previously been opened on this entity, the isFile function will be
// used to determine which file in the zip file to use. The result will
// be cached for the next time.
//
// When retrieving the entity, at least the BlobName and
// Contents fields must be populated.
func (s *Store) OpenCachedBlobFile(
	entity *mongodoc.Entity,
	fileId mongodoc.FileId,
	isFile func(f *zip.File) bool,
) (_ io.ReadCloser, err error) {
	if entity.BlobName == "" {
		// We'd like to check that the Contents field was populated
		// here but we can't because it doesn't necessarily
		// exist in the entity.
		return nil, errgo.New("provided entity does not have required fields")
	}
	zipf, ok := entity.Contents[fileId]
	if ok && !zipf.IsValid() {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	blob, size, err := s.BlobStore.Open(entity.BlobName)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open archive blob")
	}
	defer func() {
		// When there's an error, we want to close
		// the blob, otherwise we need to keep the blob
		// open because it's used by the returned Reader.
		if err != nil {
			blob.Close()
		}
	}()
	if !ok {
		// We haven't already searched the archive for the icon,
		// so find its archive now.
		zipf, err = s.findZipFile(blob, size, isFile)
		if err != nil && errgo.Cause(err) != params.ErrNotFound {
			return nil, errgo.Mask(err)
		}
	}
	// We update the content entry regardless of whether we've
	// found a file, so that the next time that serveIcon is called
	// it can know that we've already looked.
	err = s.DB.Entities().UpdateId(
		entity.URL,
		bson.D{{"$set",
			bson.D{{"contents." + string(fileId), zipf}},
		}},
	)
	if err != nil {
		return nil, errgo.Notef(err, "cannot update %q", entity.URL)
	}
	if !zipf.IsValid() {
		// We searched for the file and didn't find it.
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "")
	}

	// We know where the icon is stored. Now serve it up.
	r, err := ZipFileReader(blob, zipf)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make zip file reader")
	}
	// We return a ReadCloser that reads from the newly created
	// zip file reader, but when closed, will close the originally
	// opened blob.
	return struct {
		io.Reader
		io.Closer
	}{r, blob}, nil
}

func (s *Store) findZipFile(blob io.ReadSeeker, size int64, isFile func(f *zip.File) bool) (mongodoc.ZipFile, error) {
	zipReader, err := zip.NewReader(&readerAtSeeker{r: blob}, size)
	if err != nil {
		return mongodoc.ZipFile{}, errgo.Notef(err, "cannot read archive data")
	}
	for _, f := range zipReader.File {
		if isFile(f) {
			return NewZipFile(f)
		}
	}
	return mongodoc.ZipFile{}, params.ErrNotFound
}

// ArchiverTo can be used to archive a charm or bundle's
// contents to a writer. It is implemented by *charm.CharmArchive
// and *charm.BundleArchive.
type ArchiverTo interface {
	ArchiveTo(io.Writer) error
}

// getArchive is used to turn the current charm and bundle implementations
// into ReadSeekClosers for their corresponding archive.
func getArchive(c interface{}) (blobstore.ReadSeekCloser, error) {
	var path string
	switch c := c.(type) {
	case ArchiverTo:
		// For example: charm.CharmDir or charm.BundleDir.
		var buffer bytes.Buffer
		if err := c.ArchiveTo(&buffer); err != nil {
			return nil, errgo.Mask(err)
		}
		return nopCloser(bytes.NewReader(buffer.Bytes())), nil
	case *charm.BundleArchive:
		path = c.Path
	case *charm.CharmArchive:
		path = c.Path
	default:
		return nil, errgo.Newf("cannot get the archive for charm type %T", c)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return file, nil
}

type nopCloserReadSeeker struct {
	io.ReadSeeker
}

func (nopCloserReadSeeker) Close() error {
	return nil
}

// nopCloser returns a blobstore.ReadSeekCloser with a no-op Close method
// wrapping the provided ReadSeeker r.
func nopCloser(r io.ReadSeeker) blobstore.ReadSeekCloser {
	return nopCloserReadSeeker{r}
}
