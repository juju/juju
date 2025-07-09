// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
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

	// HTTPError is added to errors that occur when making HTTP requests.
	HTTPError = errors.ConstError("http error")
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
	tomb tomb.Tomb

	namespace string

	apiRemoteCallers apiremotecaller.APIRemoteCallers
	newBlobsClient   NewBlobsClientFunc

	clock  clock.Clock
	logger logger.Logger
}

// NewBlobRetriever creates a new BlobRetriever.
func NewBlobRetriever(apiRemoteCallers apiremotecaller.APIRemoteCallers, namespace string, newBlobsClient NewBlobsClientFunc, clock clock.Clock, logger logger.Logger) (*BlobRetriever, error) {
	w := &BlobRetriever{
		namespace:        namespace,
		newBlobsClient:   newBlobsClient,
		apiRemoteCallers: apiRemoteCallers,
		clock:            clock,
		logger:           logger,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// Report returns a map of internal state for the BlobRetriever.
func (r *BlobRetriever) Report() map[string]any {
	report := make(map[string]any)

	report["namespace"] = r.namespace

	return report
}

// Retrieve returns a reader for the blob with the given SHA256.
func (r *BlobRetriever) Retrieve(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	// Check if we're already dead or dying before we start to do anything.
	select {
	case <-r.tomb.Dying():
		return nil, -1, tomb.ErrDying
	case <-ctx.Done():
		return nil, -1, ctx.Err()
	default:
	}

	remotes, err := r.apiRemoteCallers.GetAPIRemotes()
	if err != nil {
		return nil, -1, errors.Errorf("failed to get API remotes: %w", err)
	} else if len(remotes) == 0 {
		return nil, -1, NoRemoteConnections
	}

	var (
		reader io.ReadCloser
		size   int64
	)
	for _, remote := range remotes {
		err := remote.Connection(ctx, func(connectionContext context.Context, conn api.Connection) error {
			httpClient, err := conn.RootHTTPClient()
			if err != nil {
				return errors.Errorf("failed to get root HTTP client: %w", err).Add(HTTPError)
			}

			client, err := r.newBlobsClient(httpClient.BaseURL, newHTTPClient(httpClient), r.logger)
			if err != nil {
				return errors.Errorf("failed to create object client: %w", err).Add(HTTPError)
			}

			if r.namespace == database.ControllerNS {
				tag, _ := conn.ModelTag()
				r.namespace = tag.Id()
			}

			// We don't want the connection context to affect the result of
			// the GetObject call, so we create a new context that has the
			// connection context as a child. This allows us to ignore the
			// child context when we call GetObject, but still allows us to
			// use the parent context for cancellation and deadlines.
			ctx := &scopedContext{
				parent: ctx,
				child:  connectionContext,
			}

			reader, size, err = client.GetObject(ctx, r.namespace, sha256)
			if errors.Is(err, jujuerrors.NotFound) {
				return errors.Errorf("blob %q not found: %w", sha256, err).Add(BlobNotFound)
			} else if err != nil {
				return errors.Errorf("failed to get object %q: %w", sha256, err)
			}

			// Ignore the child context for the rest of the operation.
			ctx.IgnoreChild()

			return nil
		})
		if err == nil {
			return reader, size, nil
		}

		if errors.IsOneOf(err, HTTPError, BlobNotFound) {
			r.logger.Debugf(ctx, "failed to retrieve blob %q from remote: %v", sha256, err)
			continue
		} else {
			return nil, -1, errors.Errorf("failed to retrieve blob %q from remote: %w", sha256, err)
		}
	}

	return nil, -1, BlobNotFound
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
	<-r.tomb.Dying()
	return tomb.ErrDying
}

type scopedContext struct {
	parent      context.Context
	child       context.Context
	ignoreChild int64
}

func (c *scopedContext) IgnoreChild() {
	atomic.AddInt64(&c.ignoreChild, 1)
}

func (c *scopedContext) IsChildIgnored() bool {
	return atomic.LoadInt64(&c.ignoreChild) > 0
}

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (c *scopedContext) Deadline() (deadline time.Time, ok bool) {
	return c.parent.Deadline()
}

// Done returns a channel that's closed when work done on behalf of this
// context should be canceled. Done may return nil if this context can
// never be canceled. Successive calls to Done return the same value.
// The close of the Done channel may happen asynchronously,
// after the cancel function returns.
func (c *scopedContext) Done() <-chan struct{} {
	d := make(chan struct{})
	go func() {
		for {
			select {
			case <-c.parent.Done():
				close(d)
				return
			case <-c.child.Done():
				if c.IsChildIgnored() {
					continue
				}

				close(d)
			}
		}
	}()
	return d
}

// If Done is not yet closed, Err returns nil.
func (c *scopedContext) Err() error {
	if err := c.parent.Err(); err != nil {
		return err
	}
	if c.IsChildIgnored() {
		return nil
	}
	if err := c.child.Err(); err != nil {
		return err
	}
	return nil
}

// Value returns the value associated with this context for key, or nil
// if no value is associated with key. Successive calls to Value with
// the same key returns the same result.
func (c *scopedContext) Value(key any) any {
	if v := c.parent.Value(key); v != nil {
		return v
	}
	if c.IsChildIgnored() {
		return nil
	}

	return c.child.Value(key)
}
