// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/s3client"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

const (
	// NoRemoteConnections is returned when there are no remote connections
	// available.
	NoRemoteConnections = errors.ConstError("no remote connections available")

	// BlobNotFound is returned when the requested blob is not found on any of
	// the remote connections.
	BlobNotFound = errors.ConstError("blob not found")
)

// BlobsClient is an interface for retrieving objects from an object store.
type BlobsClient interface {
	// GetObject returns a reader for the object with the given key in the
	// given bucket.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, int64, error)
}

// NewBlobsClientFunc is a function that creates a new BlobsClient.
type NewBlobsClientFunc func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error)

// BlobRetriever is responsible for retrieving blobs from remote API servers.
type BlobRetriever struct {
	catacomb catacomb.Catacomb

	namespace string

	apiRemoteCallers apiremotecaller.APIRemoteCallers
	newBlobsClient   NewBlobsClientFunc

	runner *worker.Runner

	clock  clock.Clock
	logger logger.Logger

	index uint64
}

// NewBlobRetriever creates a new BlobRetriever.
func NewBlobRetriever(apiRemoteCallers apiremotecaller.APIRemoteCallers, namespace string, newBlobsClient NewBlobsClientFunc, clock clock.Clock, logger logger.Logger) (*BlobRetriever, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "blob-retriever",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return false
		},
		Clock:  clock,
		Logger: internalworker.WrapLogger(logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	w := &BlobRetriever{
		namespace:        namespace,
		newBlobsClient:   newBlobsClient,
		apiRemoteCallers: apiRemoteCallers,
		clock:            clock,
		logger:           logger,

		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "blob-retriever",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, err
	}

	return w, nil
}

type indexMap map[uint64]struct{}

// Retrieve returns a reader for the blob with the given SHA256.
func (r *BlobRetriever) Retrieve(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	// Check if we're already dead or dying before we start to do anything.
	select {
	case <-r.catacomb.Dying():
		return nil, -1, r.catacomb.ErrDying()
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	default:
	}

	remotes := r.apiRemoteCallers.GetAPIRemotes()
	if len(remotes) == 0 {
		return nil, -1, NoRemoteConnections
	}

	indexes, result, err := r.spawn(ctx, remotes, sha256)
	if err != nil {
		return nil, -1, err
	}

	return r.collect(ctx, indexes, sha256, result)
}

func (r *BlobRetriever) spawn(ctx context.Context, remotes []apiremotecaller.RemoteConnection, sha256 string) (indexMap, chan retrievalResult, error) {
	result := make(chan retrievalResult)

	// Retrieve the blob from all the remotes concurrently.
	indexes := make(indexMap)
	for _, remote := range remotes {
		index := atomic.AddUint64(&r.index, 1)
		indexes[index] = struct{}{}

		if err := r.runner.StartWorker(ctx, name(index, sha256), func(ctx context.Context) (worker.Worker, error) {
			return newRetriever(index, remote, r.newBlobsClient, r.clock, r.logger), nil
		}); errors.Is(err, jujuerrors.AlreadyExists) {
			return nil, nil, errors.Errorf("retriever %d already exists", index)
		} else if err != nil {
			return nil, nil, err
		}

		go func(index uint64) {
			w, err := r.runner.Worker(name(index, sha256), r.catacomb.Dying())
			if err != nil {
				select {
				case <-r.catacomb.Dying():
				case <-ctx.Done():
				case result <- retrievalResult{
					index: index,
					err:   err,
				}:
				}
				return
			}

			ret := w.(*retriever)
			reader, size, err := ret.Retrieve(ctx, r.namespace, sha256)
			select {
			case <-r.catacomb.Dying():
			case <-ctx.Done():
			case result <- retrievalResult{
				index:  ret.index,
				reader: reader,
				size:   size,
				err:    err,
			}:
			}
		}(index)
	}

	return indexes, result, nil
}

func (r *BlobRetriever) collect(ctx context.Context, indexes indexMap, sha256 string, result chan retrievalResult) (_ io.ReadCloser, _ int64, err error) {
	// If the function returns an error, we want to stop all the retrievers. If
	// there is an error, we will return the retriever that was successful and
	// close the other readers. Once the reader is closed, the retriever will be
	// stopped, which will then clean up this set of requests.
	defer func() {
		if err == nil {
			return
		}

		r.stopAllRetrievers(ctx, indexes, sha256)
	}()

	// We want to run it like this so we can return the first successful result
	// and close the other readers. If we use for range over the channel, we
	// have no way to close the result.
	results := make(indexMap)
	for {
		select {
		case <-r.catacomb.Dying():
			return nil, -1, r.catacomb.ErrDying()

		case <-ctx.Done():
			return nil, -1, ctx.Err()

		case res := <-result:
			results[res.index] = struct{}{}

			// If the blob is not found on that remote, continue to the next one
			// until it is exhausted. This is a race to find it first.
			if err := res.err; errors.Is(err, BlobNotFound) {
				if len(results) == len(indexes) {
					return nil, -1, BlobNotFound
				}
				continue
			} else if err != nil {
				// If there is an error that is not BlobNotFound, return it
				return nil, -1, err
			}

			// Stop all the other retrievers, we don't want to cancel the
			// retriever that is currently being used, as that will cause the
			// reader to be closed.
			for index := range indexes {
				if index == res.index {
					continue
				}

				if err := r.runner.StopWorker(name(index, sha256)); err != nil {
					return nil, -1, errors.Errorf("failed to stop retriever %d: %w", index, err)
				}
			}

			return &retrieverReaderCloser{
				reader: res.reader,
				closer: func() {
					r.stopAllRetrievers(ctx, indexes, sha256)
				},
			}, res.size, nil
		}
	}
}

// Kill stops the BlobRetriever.
func (r *BlobRetriever) Kill() {
	r.catacomb.Kill(nil)
}

// Wait waits for the BlobRetriever to stop.
func (r *BlobRetriever) Wait() error {
	return r.catacomb.Wait()
}

func (r *BlobRetriever) loop() error {
	select {
	case <-r.catacomb.Dying():
		return r.catacomb.ErrDying()
	}
}

// stopAllRetrievers stops all the retrievers and waits for them to stop. This
// ensures that there are no dangling goroutines.
func (r *BlobRetriever) stopAllRetrievers(ctx context.Context, indexes map[uint64]struct{}, sha256 string) {
	// Kill 'Em All.
	for index := range indexes {
		if err := r.runner.StopWorker(name(index, sha256)); err != nil && !errors.Is(err, jujuerrors.NotFound) {
			r.logger.Errorf(ctx, "failed to stop retriever %d: %v", index, err)
		}
	}
}

type retriever struct {
	tomb tomb.Tomb

	index          uint64
	remote         apiremotecaller.RemoteConnection
	newBlobsClient NewBlobsClientFunc
	clock          clock.Clock
	logger         logger.Logger
	requests       chan retrievalRequest
}

func newRetriever(index uint64, remote apiremotecaller.RemoteConnection, newBlobsClient NewBlobsClientFunc, clock clock.Clock, logger logger.Logger) *retriever {
	t := &retriever{
		index:          index,
		remote:         remote,
		newBlobsClient: newBlobsClient,
		clock:          clock,
		logger:         logger,
		requests:       make(chan retrievalRequest),
	}

	t.tomb.Go(t.loop)

	return t
}

// Retrieve requests a blob from the remote API server, if there is not a remote
// connection, it will retry a number of times before returning an error. This
// is to allow time for the API connection to come up on a new HA node, or after
// a restart. If the blob isn't found or any other non-retryable error it will
// return an error right away. If the context is cancelled, it will stop
// processing the request as soon as possible.
func (t *retriever) Retrieve(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
	res := make(chan retrievalResult)
	select {
	case <-t.tomb.Dying():
		return nil, -1, t.tomb.Err()
	case t.requests <- retrievalRequest{
		ctx:       ctx,
		namespace: namespace,
		sha256:    sha256,
		result:    res,
	}:
	}

	select {
	case <-t.tomb.Dying():
		return nil, -1, t.tomb.Err()
	case res := <-res:
		return res.reader, res.size, res.err
	}
}

func (t *retriever) Kill() {
	t.tomb.Kill(nil)
}

func (t *retriever) Wait() error {
	return t.tomb.Wait()
}

func (t *retriever) loop() error {
	for {
		select {
		case <-t.tomb.Dying():
			return t.tomb.Err()
		case req := <-t.requests:
			reader, size, err := t.retrieve(req.ctx, req.namespace, req.sha256)
			select {
			case <-t.tomb.Dying():
				// If we get killed whilst attempting to return the result,
				// ensure we clean up the reader.
				if reader != nil {
					reader.Close()
				}
			case req.result <- retrievalResult{
				index:  t.index,
				reader: reader,
				size:   size,
				err:    err,
			}:
			}
		}
	}
}

func (t *retriever) retrieve(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
	ctx, cancel := context.WithCancel(t.tomb.Context(ctx))
	defer cancel()

	var (
		reader io.ReadCloser
		size   int64
	)
	err := t.remote.Connection(ctx, func(ctx context.Context, conn api.Connection) error {
		httpClient, err := conn.RootHTTPClient()
		if err != nil {
			return errors.Errorf("failed to get root HTTP client: %w", err)
		}

		client, err := t.newBlobsClient(httpClient.BaseURL, newHTTPClient(httpClient), t.logger)
		if err != nil {
			return errors.Errorf("failed to create object client: %w", err)
		}

		if namespace == database.ControllerNS {
			tag, _ := conn.ModelTag()
			namespace = tag.Id()
		}

		reader, size, err = client.GetObject(ctx, namespace, sha256)
		if errors.Is(err, jujuerrors.NotFound) {
			return errors.Errorf("blob %q not found: %w", sha256, err).Add(BlobNotFound)
		} else if err != nil {
			return errors.Errorf("failed to get object %q: %w", sha256, err)
		}
		return nil
	})
	return reader, size, err
}

type retrievalRequest struct {
	ctx       context.Context
	namespace string
	sha256    string
	result    chan<- retrievalResult
}

type retrievalResult struct {
	index  uint64
	reader io.ReadCloser
	size   int64
	err    error
}

type retrieverReaderCloser struct {
	reader io.ReadCloser
	closer func()
}

func (t *retrieverReaderCloser) Read(p []byte) (n int, err error) {
	return t.reader.Read(p)
}

func (t *retrieverReaderCloser) Close() error {
	err := t.reader.Close()
	t.closer()
	return err
}

func name(index uint64, sha256 string) string {
	return fmt.Sprintf("retriever-%s-%d", sha256, index)
}
