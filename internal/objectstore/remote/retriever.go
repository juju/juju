// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"io"

	"github.com/juju/clock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/s3client"
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

// NewObjectClientFunc is a function that creates a new BlobsClient.
type NewObjectClientFunc func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error)

// BlobRetriever is responsible for retrieving blobs from remote API servers.
type BlobRetriever struct {
	tomb tomb.Tomb

	namespace string

	apiRemoteCallers apiremotecaller.APIRemoteCallers
	newObjectClient  NewObjectClientFunc

	clock  clock.Clock
	logger logger.Logger
}

// NewBlobRetriever creates a new BlobRetriever.
func NewBlobRetriever(apiRemoteCallers apiremotecaller.APIRemoteCallers, namespace string, newObjectClient NewObjectClientFunc, clock clock.Clock, logger logger.Logger) *BlobRetriever {
	w := &BlobRetriever{
		namespace:        namespace,
		newObjectClient:  newObjectClient,
		apiRemoteCallers: apiRemoteCallers,
		clock:            clock,
		logger:           logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// GetBySHA256 returns a reader for the blob with the given SHA256.
func (r *BlobRetriever) Retrieve(ctx context.Context, sha256 string) (_ io.ReadCloser, _ int64, err error) {
	// Check if we're already dead or dying before we start to do anything.
	select {
	case <-r.tomb.Dying():
		return nil, -1, tomb.ErrDying
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	default:
	}

	remotes := r.apiRemoteCallers.GetAPIRemotes()
	if len(remotes) == 0 {
		return nil, -1, NoRemoteConnections
	}

	result := make(chan retrievalResult)

	// Register all the retrievers, we can then reference them by index later on.
	retrievers := make([]*retriever, len(remotes))
	for index, remote := range remotes {
		retrievers[index] = newRetriever(index, remote, r.newObjectClient, r.clock, r.logger)
	}

	// Tie the context to the tomb so that we can stop all the retrievers when
	// the tomb is killed.
	ctx = r.tomb.Context(ctx)

	// Retrieve the blob from all the remotes concurrently.
	for _, ret := range retrievers {
		go func(ret *retriever) {
			reader, size, err := ret.Retrieve(ctx, r.namespace, sha256)
			select {
			case <-ctx.Done():
			case result <- retrievalResult{
				index:  ret.index,
				reader: reader,
				size:   size,
				err:    err,
			}:
			}
		}(ret)
	}

	// If the function returns an error, we want to stop all the retrievers. If
	// there is an error, we will return the retriever that was successful and
	// close the other readers. Once the reader is closed, the retriever will be
	// stopped, which will then clean up this set of requests.
	defer func() {
		if err == nil {
			return
		}

		r.stopAllRetrievers(ctx, retrievers)
	}()

	// We want to run it like this so we can return the first successful result
	// and close the other readers. If we use for range over the channel, we
	// have no way to close the result.
	results := make(map[int]struct{})
	for {
		select {
		case <-r.tomb.Dying():
			return nil, -1, tomb.ErrDying

		case <-ctx.Done():
			return nil, -1, ctx.Err()

		case res := <-result:
			results[res.index] = struct{}{}

			// If the blob is not found on that remote, continue to the next one
			// until it is exhausted. This is a race to find it first.
			if err := res.err; errors.Is(err, BlobNotFound) {
				if len(results) == len(remotes) {
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
			for _, retriever := range retrievers {
				if retriever.index == res.index {
					continue
				}

				retriever.Kill()

				// Don't wait for them to stop, when closing the reader, we will
				// wait for them to stop then. We just want them to stop
				// processing the request as soon as possible.
			}

			return &retrieverReaderCloser{
				reader: res.reader,
				closer: func() {
					r.stopAllRetrievers(ctx, retrievers)
				},
			}, res.size, nil
		}
	}
}

// Kill stops the BlobRetriever.
func (r *BlobRetriever) Kill() {
	r.tomb.Kill(nil)
}

// Wait waits for the BlobRetriever to stop.
func (r *BlobRetriever) Wait() error {
	return r.tomb.Wait()
}

func (r *BlobRetriever) loop() error {
	select {
	case <-r.tomb.Dying():
		return tomb.ErrDying
	}
}

// stopAllRetrievers stops all the retrievers and waits for them to stop. This
// ensures that there are no dangling goroutines.
func (r *BlobRetriever) stopAllRetrievers(ctx context.Context, retrievers []*retriever) {
	// Kill 'Em All.
	for _, retriever := range retrievers {
		retriever.Kill()
		// Wait for them to stop, we don't want to leave any hanging.
		if err := retriever.Wait(); err != nil {
			r.logger.Errorf(ctx, "failed to stop blob retriever: %v", err)
		}
	}
}

type retriever struct {
	tomb tomb.Tomb

	index           int
	remote          apiremotecaller.RemoteConnection
	newObjectClient NewObjectClientFunc
	clock           clock.Clock
	logger          logger.Logger
}

func newRetriever(index int, remote apiremotecaller.RemoteConnection, newObjectClient NewObjectClientFunc, clock clock.Clock, logger logger.Logger) *retriever {
	t := &retriever{
		index:           index,
		remote:          remote,
		newObjectClient: newObjectClient,
		clock:           clock,
		logger:          logger,
	}

	t.tomb.Go(func() error {
		<-t.tomb.Dying()
		return tomb.ErrDying
	})

	return t
}

// Retrieve requests a blob from the remote API server, if there is not a remote
// connection, it will retry a number of times before returning an error. This
// is to allow time for the API connection to come up on a new HA node, or after
// a restart. If the blob isn't found or any other non-retryable error it will
// return an error right away. If the context is cancelled, it will stop
// processing the request as soon as possible.
func (t *retriever) Retrieve(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		reader io.ReadCloser
		size   int64
	)
	err := t.remote.Connection(ctx, func(ctx context.Context, conn api.Connection) error {
		go func() {
			defer cancel()

			// If the connection is broken, we want to stop processing the
			// request as soon as possible.
			select {
			case <-t.tomb.Dying():
			case <-ctx.Done():
			case <-conn.Broken():
			}
		}()

		httpClient, err := conn.RootHTTPClient()
		if err != nil {
			return errors.Errorf("failed to get root HTTP client: %v", err)
		}

		client, err := t.newObjectClient(httpClient.BaseURL, newHTTPClient(httpClient), t.logger)
		if err != nil {
			return errors.Errorf("failed to create object client: %v", err)
		}

		if namespace == database.ControllerNS {
			tag, _ := conn.ModelTag()
			namespace = tag.Id()
		}

		reader, size, err = client.GetObject(ctx, namespace, sha256)
		return err
	})
	return reader, size, err
}

func (t *retriever) Kill() {
	t.tomb.Kill(nil)
}

func (t *retriever) Wait() error {
	return t.tomb.Wait()
}

type retrievalResult struct {
	index  int
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
