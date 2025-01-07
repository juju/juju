// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
	"gopkg.in/httprequest.v1"
	"gopkg.in/tomb.v2"

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

	// NoRemoteConnection is returned when there is no remote connection
	// available.
	NoRemoteConnection = errors.ConstError("no remote connection available")

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
		apiRemoteCallers: apiRemoteCallers,
		newObjectClient:  newObjectClient,
		clock:            clock,
		logger:           logger,
	}

	w.tomb.Go(w.loop)

	return w
}

type retrievalResult struct {
	reader io.ReadCloser
	size   int64
	err    error
}

// GetBySHA256 returns a reader for the blob with the given SHA256.
func (r *BlobRetriever) RetrieveBlobFromRemote(ctx context.Context, sha256 string) (io.ReadCloser, int64, error) {
	conns := r.apiRemoteCallers.GetAPIRemotes()
	if len(conns) == 0 {
		return nil, -1, NoRemoteConnections
	}

	result := make(chan retrievalResult, len(conns))

	// This will cancel the context when the tomb is dying, or when the passed
	// context is cancelled.
	ctx, cancel := r.scopedContext(ctx)

	// This cancel is cancelling the reader.
	defer cancel()

	for _, conn := range conns {
		go func() {
			reader, size, err := r.retrieveBlobFromRemote(ctx, conn, sha256)
			select {
			case <-ctx.Done():
				return
			case result <- retrievalResult{reader: reader, size: size, err: err}:
			}
		}()
	}

	// We want to run it like this so we can return the first successful result
	// and close the other readers. If we use for range over the channel, we
	// have no way to close the result.
	for i := 0; i < len(conns); i++ {
		select {
		case <-ctx.Done():
			return nil, -1, ctx.Err()
		case <-r.tomb.Dying():
			return nil, -1, tomb.ErrDying
		case res := <-result:
			// If the blob is not found on that remote, continue to the next one
			// until it is exhausted. This is a race to find it first.
			if err := res.err; errors.Is(err, BlobNotFound) {
				continue
			} else if err != nil {
				return nil, -1, err
			}
			return res.reader, res.size, nil
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
	select {
	case <-r.tomb.Dying():
		return tomb.ErrDying
	}
}

func (r *BlobRetriever) retrieveBlobFromRemote(ctx context.Context, remote apiremotecaller.RemoteConnection, sha256 string) (io.ReadCloser, int64, error) {
	var (
		reader io.ReadCloser
		size   int64
	)
	if err := retry.Call(retry.CallArgs{
		Func: func() error {
			conn, ok := remote.Connection()
			if !ok {
				return NoRemoteConnection
			}

			httpClient, err := conn.RootHTTPClient()
			if err != nil {
				return err
			}

			client, err := r.newObjectClient(httpClient.BaseURL, newHTTPClient(httpClient), r.logger)
			if err != nil {
				return err
			}

			namespace := r.namespace
			if r.namespace == database.ControllerNS {
				tag, _ := conn.ModelTag()
				namespace = tag.Id()
			}

			reader, size, err = client.GetObject(ctx, namespace, sha256)
			return err
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, NoRemoteConnection)
		},
		NotifyFunc: func(lastError error, attempt int) {
			r.logger.Infof("Failed to retrieve blob from remote: %v (attempt %d)", lastError, attempt)
		},
		Clock:       r.clock,
		Stop:        ctx.Done(),
		Attempts:    20,
		Delay:       time.Millisecond * 500,
		MaxDelay:    time.Second * 5,
		MaxDuration: time.Minute,
	}); err != nil {
		return nil, -1, err
	}
	return reader, size, nil

}

func (r *BlobRetriever) scopedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(r.tomb.Context(ctx))
}

// NewObjectClient returns a new client based on the supplied dependencies.
// This only provides a read only session to the object store. As this is
// intended to be used by the unit, there is never an expectation that the unit
// will write to the object store.
func NewObjectClient(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
	session, err := s3client.NewS3Client(ensureHTTPS(url), client, s3client.AnonymousCredentials{}, logger)
	if err != nil {
		return nil, err
	}

	return s3client.NewBlobsS3Client(session), nil
}

// httpClient is a shim around a shim. The httprequest.Client is a shim around
// the stdlib http.Client. This is just asinine. The httprequest.Client should
// be ripped out and replaced with the stdlib http.Client.
type httpClient struct {
	client *httprequest.Client
}

func newHTTPClient(client *httprequest.Client) *httpClient {
	return &httpClient{
		client: client,
	}
}

func (c *httpClient) Do(req *http.Request) (*http.Response, error) {
	var res *http.Response
	err := c.client.Do(req.Context(), req, &res)
	return res, err
}

// ensureHTTPS takes a URI and ensures that it is a HTTPS URL.
func ensureHTTPS(address string) string {
	if strings.HasPrefix(address, "https://") {
		return address
	}
	if strings.HasPrefix(address, "http://") {
		return strings.Replace(address, "http://", "https://", 1)
	}
	return "https://" + address
}
