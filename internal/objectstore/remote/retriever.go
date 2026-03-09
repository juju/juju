// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"io"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
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

// ModelTagGetter is an interface for getting the model tag from an API
// connection.
type ModelTagGetter interface {
	// ModelTag returns the model tag for the current connection, if available.
	ModelTag() (names.ModelTag, bool)
}

// GetNamespaceFunc is a function that returns the namespace to use for
// retrieving blobs from the remote API, given a ModelTagGetter to get the model
// tag from the API connection.
type GetNamespaceFunc func(ModelTagGetter) (string, error)

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

	namespace       string
	namespaceGetter GetNamespaceFunc

	apiRemoteCallers apiremotecaller.APIRemoteCallers
	newBlobsClient   NewBlobsClientFunc

	clock  clock.Clock
	logger logger.Logger
}

// NewControllerBlobRetriever creates a new BlobRetriever.
func NewControllerBlobRetriever(apiRemoteCallers apiremotecaller.APIRemoteCallers, newBlobsClient NewBlobsClientFunc, clock clock.Clock, logger logger.Logger) (*BlobRetriever, error) {
	w := &BlobRetriever{
		namespace: database.ControllerNS,
		namespaceGetter: func(mtg ModelTagGetter) (string, error) {
			tag, ok := mtg.ModelTag()
			if !ok {
				return "", errors.Errorf("missing model tag when using controller namespace")
			}
			return tag.Id(), nil
		},
		newBlobsClient:   newBlobsClient,
		apiRemoteCallers: apiRemoteCallers,
		clock:            clock,
		logger:           logger,
	}

	w.tomb.Go(w.loop)

	return w, nil
}

// NewModelBlobRetriever creates a new BlobRetriever.
func NewModelBlobRetriever(apiRemoteCallers apiremotecaller.APIRemoteCallers, namespace string, newBlobsClient NewBlobsClientFunc, clock clock.Clock, logger logger.Logger) (*BlobRetriever, error) {
	w := &BlobRetriever{
		namespace: namespace,
		namespaceGetter: func(mtg ModelTagGetter) (string, error) {
			return namespace, nil
		},
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
func (r *BlobRetriever) Retrieve(ctx context.Context, sha256 string, controllerIDHints []string) (io.ReadCloser, int64, error) {
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

	// If we have controller ID hints attempt to prioritize remotes that match
	// the hints. Otherwise shuffle the other remotes to avoid always hitting
	// the same one first.
	if len(controllerIDHints) > 0 {
		remotes = prioritizeRemotes(remotes, controllerIDHints)
	} else {
		remotes = shuffleRemotes(remotes)
	}

	return r.retrieveFromRemotes(ctx, remotes, sha256)
}

func (r *BlobRetriever) retrieveFromRemotes(ctx context.Context, remotes []apiremotecaller.RemoteConnection, sha256 string) (io.ReadCloser, int64, error) {
	// Iterate over the remotes and try to retrieve the blob from each one.
	var errs []string
	for _, remote := range remotes {
		reader, size, err := r.retrieve(ctx, remote, sha256)
		if errors.Is(err, HTTPError) {
			errs = append(errs, err.Error())
			r.logger.Debugf(ctx, "failed to retrieve blob %q from remote: %v", sha256, err)
			continue
		} else if errors.Is(err, BlobNotFound) {
			continue
		} else if err != nil {
			return nil, -1, errors.Errorf("failed to retrieve blob %q from remote: %w", sha256, err)
		}

		return reader, size, nil
	}

	return nil, -1, errors.Errorf(`failed to retrieve %q: %s`, sha256, strings.Join(errs, ",")).Add(BlobNotFound)
}

func (r *BlobRetriever) retrieve(ctx context.Context, remote apiremotecaller.RemoteConnection, sha256 string) (io.ReadCloser, int64, error) {
	var reader io.ReadCloser
	var size int64

	err := remote.Connection(ctx, func(connectionContext context.Context, conn api.Connection) error {
		httpClient, err := conn.SimpleHTTPClient()
		if err != nil {
			return errors.Errorf("failed to get root HTTP client: %w", err).Add(HTTPError)
		}

		client, err := r.newBlobsClient(httpClient.BaseURL(), newHTTPClient(httpClient), r.logger)
		if err != nil {
			return errors.Errorf("failed to create object client: %w", err).Add(HTTPError)
		}

		ns, err := r.namespaceGetter(conn)
		if err != nil {
			return errors.Errorf("failed to get namespace: %w", err).Add(HTTPError)
		}

		ctx := &scopedContext{
			parent: ctx,
			child:  connectionContext,
		}

		reader, size, err = client.GetObject(ctx, ns, sha256)
		if errors.Is(err, jujuerrors.NotFound) {
			return errors.Errorf("blob %q not found: %w", sha256, err).Add(BlobNotFound)
		} else if err != nil {
			return errors.Errorf("failed to get object %q: %w", sha256, err)
		}

		ctx.IgnoreChild()

		return nil
	})

	return reader, size, err
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

// Shuffle the remotes to avoid always hitting the same one first. This copies
// the slice before shuffling to avoid modifying the original slice, which may
// be shared with other parts of the code.
func shuffleRemotes(remotes []apiremotecaller.RemoteConnection) []apiremotecaller.RemoteConnection {
	if len(remotes) <= 1 {
		return remotes
	}

	shuffled := make([]apiremotecaller.RemoteConnection, len(remotes))
	copy(shuffled, remotes)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled
}

func prioritizeRemotes(remotes []apiremotecaller.RemoteConnection, hints []string) []apiremotecaller.RemoteConnection {
	if len(remotes) <= 1 {
		return remotes
	}

	set := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		set[hint] = struct{}{}
	}

	prioritize, fallback := partition(remotes, func(remote apiremotecaller.RemoteConnection) bool {
		_, found := set[remote.ControllerID()]
		return found
	})

	return append(shuffleRemotes(prioritize), shuffleRemotes(fallback)...)
}

// partition splits the slice into two slices based on the provided function.
// The first slice contains all elements for which the function returns true, and
// the second slice contains all elements for which the function returns false.
func partition[S ~[]E, E any](s S, f func(E) bool) (S, S) {
	if len(s) == 0 {
		return s, s
	}

	var a, b S
	for _, v := range s {
		if f(v) {
			a = append(a, v)
		} else {
			b = append(b, v)
		}
	}
	return a, b
}

// scopedContext is a context that allows us to ignore the child context
// when retrieving objects from the remote API. This is vital, because we
// want to be able to cancel the retrieval operation if the get object process
// fails, but we don't want the cancellation to propagate once we've received
// the object. We're not buffering the object reader, so any cancellation
// will cause the reader to be closed, which is not what we want.
type scopedContext struct {
	parent      context.Context
	child       context.Context
	ignoreChild atomic.Bool
}

// IgnoreChild sets the ignoreChild flag to true, which means that the child
// context will be ignored when retrieving objects from the remote API.
func (c *scopedContext) IgnoreChild() {
	c.ignoreChild.Store(true)
}

// IsChildIgnored returns true if the child context is ignored, which means
// that the child context will not be consulted when retrieving objects from the
// remote API.
func (c *scopedContext) IsChildIgnored() bool {
	return c.ignoreChild.Load()
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

	closeDone := sync.OnceFunc(func() {
		close(d)
	})

	parentDone := c.parent.Done()
	childDone := c.child.Done()

	go func() {
		for {
			select {
			case <-parentDone:
				closeDone()
				return
			case <-childDone:
				// If the child context is ignored, we don't want to close
				// the done channel, because we don't want to propagate the
				// cancellation to the caller.
				if c.IsChildIgnored() {
					// The child channel is closed, disable this select arm to
					// avoid a tight loop while we continue to wait on parent
					// cancellation.
					childDone = nil
					continue
				}

				closeDone()
				return
			}
		}
	}()
	return d
}

// Err returns nil if channel returned by Done() is not yet closed.
func (c *scopedContext) Err() error {
	if err := c.parent.Err(); err != nil {
		return err
	}

	// If the child context is ignored, we don't want to return its error.
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

	// If the child context is ignored, we don't want to return any value from
	// it.
	if c.IsChildIgnored() {
		return nil
	}

	return c.child.Value(key)
}
