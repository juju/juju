package objectstoredrainer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"gopkg.in/tomb.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
)

// MetadataService is the interface that is used to get metadata from the
// object store. This is used to list all files in the object store that
// need to be drained from the file backed object store to the s3 object store.
type MetadataService interface {
	// ListMetadata returns the metadata for all files in the object store.
	ListMetadata(ctx context.Context) ([]objectstore.Metadata, error)
}

type drainWorker struct {
	tomb tomb.Tomb

	selectFileHash SelectFileHashFunc
	fileSystem     HashFileSystemAccessor
	client         objectstore.Client

	metadataService MetadataService

	rootBucket string
	namespace  string

	logger logger.Logger
}

func newDrainWorker(
	fileSystem HashFileSystemAccessor,
	client objectstore.Client,
	metadataService MetadataService,
	rootBucket, namespace string,
	selectFileHash SelectFileHashFunc,
	logger logger.Logger,
) *drainWorker {
	w := &drainWorker{
		fileSystem:      fileSystem,
		client:          client,
		metadataService: metadataService,
		rootBucket:      rootBucket,
		namespace:       namespace,
		selectFileHash:  selectFileHash,
		logger:          logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill kills the worker. This will stop the worker from processing any
// further requests and will wait for the worker to finish.
func (w *drainWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to finish. It will return an error if the
// worker was killed with an error, or if the worker encountered an error
// while running.
func (w *drainWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *drainWorker) Report() map[string]any {
	return map[string]any{
		"namespace":  w.namespace,
		"rootBucket": w.rootBucket,
	}
}

func (w *drainWorker) loop() error {
	ctx := w.tomb.Context(context.Background())

	// Drain any files from the file object store to the s3 object store.
	// This will locate any files from the metadata service that are not
	// present in the s3 object store and copy them over.
	metadata, err := w.metadataService.ListMetadata(ctx)
	if err != nil {
		return errors.Errorf("listing metadata for draining: %w", err)
	}

	for _, m := range metadata {
		hash := w.selectFileHash(m)

		if err := w.drainFile(ctx, m.Path, hash, m.Size); err != nil {
			// This will crash the s3ObjectStore worker if this is a fatal
			// error. We don't want to continue processing if we can't drain
			// the files to the s3 object store.

			return errors.Errorf("draining file %q to s3 object store: %w", m.Path, err)
		}
	}

	return nil
}

func (w *drainWorker) drainFile(ctx context.Context, path, hash string, metadataSize int64) error {
	// If the file isn't on the file backed object store, then we can skip it.
	// It's expected that this has already been drained to the s3 object store.
	if err := w.fileSystem.HashExists(ctx, hash); errors.Is(err, coreerrors.NotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("checking if file %q exists in file object store: %w", path, err)
	}

	// If the file is already in the s3 object store, then we can skip it.
	// Note: we want to check the s3 object store each request, just in
	// case the file was added to the s3 object store while we were
	// draining the files.
	if err := w.objectAlreadyExists(ctx, hash); err != nil && !errors.Is(err, coreerrors.NotFound) {
		return errors.Errorf("checking if file %q exists in s3 object store: %w", path, err)
	} else if err == nil {
		// File already contains the hash, so we can skip it.
		w.logger.Tracef(ctx, "file %q already exists in s3 object store, skipping", path)
		return nil
	}

	w.logger.Debugf(ctx, "draining file %q to s3 object store", path)

	// Grab the file from the file backed object store and drain it to the
	// s3 object store.
	reader, fileSize, err := w.fileSystem.GetByHash(ctx, hash)
	if err != nil {
		// The file doesn't exist in the file backed object store, but also
		// doesn't exist in the s3 object store. This is a problem, so we
		// should skip it.
		if errors.Is(err, coreerrors.NotFound) {
			w.logger.Warningf(ctx, "file %q doesn't exist in file object store, unable to drain", path)
			return nil
		}
		return errors.Errorf("getting file %q from file object store: %w", path, err)
	}

	// Ensure we close the reader when we're done.
	defer reader.Close()

	// If the file size doesn't match the metadata size, then the file is
	// potentially corrupt, so we should skip it.
	if fileSize != metadataSize {
		w.logger.Warningf(ctx, "file %q has a size mismatch, unable to drain", path)
		return nil
	}

	// We need to compute the sha256 hash here, juju by default uses SHA384,
	// but s3 defaults to SHA256.
	// If the reader is a Seeker, then we can seek back to the beginning of
	// the file, so that we can read it again.
	s3Reader, s3EncodedHash, err := w.computeS3Hash(reader)
	if err != nil {
		return errors.Capture(err)
	}

	// We can drain the file to the s3 object store.
	err = w.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.PutObject(ctx, w.rootBucket, w.filePath(hash), s3Reader, s3EncodedHash)
		if err != nil {
			return errors.Errorf("putting file %q to s3 object store: %w", path, err)
		}
		return nil
	})
	if errors.Is(err, coreerrors.AlreadyExists) {
		// File already contains the hash, so we can skip it.
		return w.removeDrainedFile(ctx, hash)
	} else if err != nil {
		return errors.Capture(err)
	}

	// We can remove the file from the file backed object store, because it
	// has been successfully drained to the s3 object store.
	if err := w.removeDrainedFile(ctx, hash); err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (w *drainWorker) computeS3Hash(reader io.Reader) (io.Reader, string, error) {
	s3Hash := sha256.New()

	// This is an optimization for the case where the reader is a Seeker. We
	// can seek back to the beginning of the file, so that we can read it
	// again, without having to copy the entire file into memory.
	if seekReader, ok := reader.(io.Seeker); ok {
		if _, err := io.Copy(s3Hash, reader); err != nil {
			return nil, "", errors.Errorf("computing hash: %w", err)
		}

		if _, err := seekReader.Seek(0, io.SeekStart); err != nil {
			return nil, "", errors.Errorf("seeking back to start: %w", err)
		}

		return reader, base64.StdEncoding.EncodeToString(s3Hash.Sum(nil)), nil
	}

	// If the reader is not a Seeker, then we need to copy the entire file
	// into memory, so that we can compute the hash.
	memReader := new(bytes.Buffer)
	if _, err := io.Copy(io.MultiWriter(s3Hash, memReader), reader); err != nil {
		return nil, "", errors.Errorf("computing hash: %w", err)
	}

	return memReader, base64.StdEncoding.EncodeToString(s3Hash.Sum(nil)), nil
}

func (w *drainWorker) objectAlreadyExists(ctx context.Context, hash string) error {
	if err := w.client.Session(ctx, func(ctx context.Context, s objectstore.Session) error {
		err := s.ObjectExists(ctx, w.rootBucket, w.filePath(hash))
		return errors.Capture(err)
	}); err != nil {
		return errors.Errorf("checking if file %q exists in s3 object store: %w", hash, err)
	}
	return nil
}

func (w *drainWorker) removeDrainedFile(ctx context.Context, hash string) error {
	if err := w.fileSystem.DeleteByHash(ctx, hash); err != nil {
		// If we're unable to remove the file from the file backed object
		// store, then we should log a warning, but continue processing.
		// This is not a terminal case, we can continue processing.
		w.logger.Warningf(ctx, "unable to remove file %q from file object store: %v", hash, err)
		return nil
	}
	return nil
}

func (w *drainWorker) filePath(hash string) string {
	return fmt.Sprintf("%s/%s", w.namespace, hash)
}
