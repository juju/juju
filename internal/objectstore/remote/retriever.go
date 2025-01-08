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
	"github.com/juju/loggo"
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
		newObjectClient:  newObjectClient,
		apiRemoteCallers: apiRemoteCallers,
		clock:            clock,
		logger:           logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// GetBySHA256 returns a reader for the blob with the given SHA256.
func (r *BlobRetriever) RetrieveBlobFromRemote(ctx context.Context, sha256 string) (_ io.ReadCloser, _ int64, err error) {
	remotes := r.apiRemoteCallers.GetAPIRemotes()
	if len(remotes) == 0 {
		return nil, -1, NoRemoteConnections
	}

	// Tie the context to the tomb so that we can stop all the tasks when the
	// tomb is killed.
	ctx = r.tomb.Context(ctx)

	result := make(chan retrievalResult, len(remotes))

	// Register all the tasks, we can then reference them by index later on.
	tasks := make([]*task, len(remotes))
	for index, remote := range remotes {
		tasks[index] = newTask(index, remote, r.newObjectClient, r.clock, r.logger)
	}

	// Retrieve the blob from all the remotes concurrently.
	for _, task := range tasks {
		r.tomb.Go(func() error {
			reader, size, err := task.Retrieve(ctx, r.namespace, sha256)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case result <- retrievalResult{
				index:  task.index,
				reader: reader,
				size:   size,
				err:    err,
			}:
				return nil
			}
		})
	}

	// If the function returns an error, we want to stop all the tasks. If there
	// is an error, we will return the task that was successful and close the
	// other readers. Once the reader is closed, the task will be stopped, which
	// will then clean up this set of requests.
	defer func() {
		if err != nil {
			r.stopAllTasks(tasks)
		}
	}()

	// We want to run it like this so we can return the first successful result
	// and close the other readers. If we use for range over the channel, we
	// have no way to close the result.
	for i := 0; i < len(remotes); i++ {
		select {
		case <-ctx.Done():
			return nil, -1, ctx.Err()
		case res := <-result:
			// If the blob is not found on that remote, continue to the next one
			// until it is exhausted. This is a race to find it first.
			if err := res.err; errors.Is(err, BlobNotFound) {
				continue
			} else if err != nil {
				return nil, -1, err
			}

			// Stop all the other tasks!
			r.stopAllTasksExcept(tasks, res.index)

			return &taskReaderCloser{
				reader: res.reader,
				closer: func() {
					// Stop all the tasks when the reader is closed.
					// This should ensure that we don't have any hanging
					// connections.
					r.stopAllTasks(tasks)
				},
			}, res.size, nil
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

func (r *BlobRetriever) stopAllTasks(tasks []*task) {
	for _, task := range tasks {
		task.Kill()
		if err := task.Wait(); err != nil {
			r.logger.Errorf("Failed to stop task: %v", err)
		}
	}
}

func (r *BlobRetriever) stopAllTasksExcept(tasks []*task, index int) {
	for _, task := range tasks {
		if task.Index() == index {
			continue
		}

		task.Kill()
		if err := task.Wait(); err != nil {
			r.logger.Errorf("Failed to stop task: %v", err)
		}
	}
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

type retrievalResult struct {
	index  int
	reader io.ReadCloser
	size   int64
	err    error
}

type task struct {
	tomb tomb.Tomb

	index           int
	remote          apiremotecaller.RemoteConnection
	newObjectClient NewObjectClientFunc
	clock           clock.Clock
	logger          logger.Logger
}

func newTask(index int, remote apiremotecaller.RemoteConnection, newObjectClient NewObjectClientFunc, clock clock.Clock, logger logger.Logger) *task {
	t := &task{
		index:           index,
		remote:          remote,
		newObjectClient: newObjectClient,
		clock:           clock,
		logger:          logger,
	}

	t.tomb.Go(t.loop)

	return t
}

func (t *task) Retrieve(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
	ctx = t.tomb.Context(ctx)

	loggo.GetLogger("***").Criticalf("Retrieve %v %v", namespace, sha256)

	var (
		reader io.ReadCloser
		size   int64
	)
	if err := retry.Call(retry.CallArgs{
		Func: func() error {
			conn, ok := t.remote.Connection()
			if !ok {
				return NoRemoteConnection
			}

			httpClient, err := conn.RootHTTPClient()
			if err != nil {
				return err
			}

			client, err := t.newObjectClient(httpClient.BaseURL, newHTTPClient(httpClient), t.logger)
			if err != nil {
				return err
			}

			if namespace == database.ControllerNS {
				tag, _ := conn.ModelTag()
				namespace = tag.Id()
			}

			loggo.GetLogger("***").Criticalf(">>> %v %v %v", httpClient.BaseURL, namespace, sha256)

			reader, size, err = client.GetObject(ctx, namespace, sha256)
			return err
		},
		IsFatalError: func(err error) bool {
			return !errors.Is(err, NoRemoteConnection)
		},
		NotifyFunc: func(lastError error, attempt int) {
			t.logger.Infof("Failed to retrieve blob from remote: %v (attempt %d)", lastError, attempt)
		},
		Clock:       t.clock,
		Stop:        ctx.Done(),
		Attempts:    50,
		Delay:       time.Second,
		MaxDelay:    time.Minute,
		BackoffFunc: retry.ExpBackoff(time.Second, time.Second*10, 1.5, true),
	}); err != nil {
		return nil, -1, err
	}
	return reader, size, nil
}

func (t *task) Kill() {
	t.tomb.Kill(nil)
}

func (t *task) Wait() error {
	return t.tomb.Wait()
}

func (t *task) Index() int {
	return t.index
}

func (t *task) loop() error {
	select {
	case <-t.tomb.Dying():
		return tomb.ErrDying
	}
}

type taskReaderCloser struct {
	reader io.ReadCloser
	closer func()
}

func (t *taskReaderCloser) Read(p []byte) (n int, err error) {
	return t.reader.Read(p)
}

func (t *taskReaderCloser) Close() error {
	err := t.reader.Close()
	t.closer()
	return err
}
