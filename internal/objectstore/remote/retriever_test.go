// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"bytes"
	"context"
	"fmt"
	io "io"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

type retrieverSuite struct {
	testhelpers.IsolationSuite

	remoteCallers    *MockAPIRemoteCallers
	remoteConnection *MockRemoteConnection
	apiConnection    *MockConnection
	client           *MockBlobsClient
	clock            *MockClock
}

func TestRetrieverSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &retrieverSuite{})
}

func (s *retrieverSuite) TestRetrieverWithNoAPIRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return(nil)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, NoRemoteConnections)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverAlreadyKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)

	workertest.CleanKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	workertest.CheckKilled(c, ret)
}

func (s *retrieverSuite) TestRetrieverAlreadyContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, _, err := ret.Retrieve(ctx, "foo")
	c.Assert(err, tc.ErrorIs, context.Canceled)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	b := io.NopCloser(bytes.NewBufferString("hello world"))
	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").Return(b, int64(11), nil)

	s.apiConnection.EXPECT().RootHTTPClient().Return(client, nil)

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		return fn(ctx, s.apiConnection)
	})
	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	readerCloser, size, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that the reader is closed, otherwise the retriever will leak.
	// You can test this, by commenting out this line!
	defer readerCloser.Close()

	result, err := io.ReadAll(readerCloser)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []byte("hello world"))
	c.Check(size, tc.Equals, int64(11))

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesRace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	b := io.NopCloser(bytes.NewBufferString("hello world"))

	// Ensure the first one blocks until the second one is called.

	done := make(chan struct{})
	started := make(chan struct{})

	fns := []func(context.Context, string, string) (io.ReadCloser, int64, error){
		func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
			select {
			case <-started:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("timed out waiting for started")
			}

			select {
			case <-done:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("timed out waiting for done")
			}

			select {
			case <-ctx.Done():
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("timed out waiting for context to be done")
			}
			return nil, 0, ctx.Err()
		},
		func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
			defer close(done)

			select {
			case <-started:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("timed out waiting for started")
			}

			return b, int64(11), nil
		},
	}

	// Shuffle the functions to ensure we detect any issues that are dependant
	// on order.
	rand.Shuffle(len(fns), func(i, j int) {
		fns[i], fns[j] = fns[j], fns[i]
	})

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[0]),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[1]),
	)

	var attempts int64
	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		n := atomic.AddInt64(&attempts, 1)
		if n == 2 {
			close(started)
		}
		return client, nil
	}).Times(2)

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		return fn(ctx, s.apiConnection)
	}).Times(2)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	readerCloser, size, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that the reader is closed, otherwise the retriever will leak.
	// You can test this, by commenting out this line!
	defer readerCloser.Close()

	result, err := io.ReadAll(readerCloser)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []byte("hello world"))
	c.Check(size, tc.Equals, int64(11))

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesRaceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	b := io.NopCloser(bytes.NewBufferString("hello world"))

	started := make(chan struct{})

	notFound := func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
		select {
		case <-started:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out waiting for started")
		}
		return nil, 0, jujuerrors.NotFound
	}

	fns := []func(context.Context, string, string) (io.ReadCloser, int64, error){
		notFound,
		notFound,
		func(ctx context.Context, s1, s2 string) (io.ReadCloser, int64, error) {
			select {
			case <-started:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("timed out waiting for started")
			}
			return b, int64(11), nil
		},
	}

	// Shuffle the functions to ensure we detect any issues that are dependant
	// on order.
	rand.Shuffle(len(fns), func(i, j int) {
		fns[i], fns[j] = fns[j], fns[i]
	})

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[0]),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[1]),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fns[2]),
	)

	var attempts int64
	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		n := atomic.AddInt64(&attempts, 1)
		if n == 3 {
			close(started)
		}
		return client, nil
	}).Times(3)

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		return fn(ctx, s.apiConnection)
	}).Times(3)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	readerCloser, size, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that the reader is closed, otherwise the retriever will leak.
	// You can test this, by commenting out this line!
	defer readerCloser.Close()

	result, err := io.ReadAll(readerCloser)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []byte("hello world"))
	c.Check(size, tc.Equals, int64(11))

	c.Assert(atomic.LoadInt64(&attempts), tc.Equals, int64(3))

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").Return(nil, 0, jujuerrors.NotFound).Times(3)

	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		return client, nil
	}).Times(3)

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		return fn(ctx, s.apiConnection)
	}).Times(3)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, BlobNotFound)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWithAPIRemotesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	client := &httprequest.Client{
		BaseURL: "http://example.com",
	}

	started := make(chan struct{})

	fail := func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
		select {
		case <-started:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out waiting for started")
		}
		return nil, 0, fmt.Errorf("boom")
	}

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
			select {
			case <-started:
			case <-time.After(testhelpers.LongWait):
				c.Fatalf("timed out waiting for started")
			}
			return nil, 0, BlobNotFound
		}),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "foo").DoAndReturn(fail).MaxTimes(2),
	)

	var attempts int64
	s.apiConnection.EXPECT().RootHTTPClient().DoAndReturn(func() (*httprequest.Client, error) {
		n := atomic.AddInt64(&attempts, 1)
		if n == 3 {
			close(started)
		}
		return client, nil
	}).Times(3)

	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		return fn(ctx, s.apiConnection)
	}).Times(3)

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
		s.remoteConnection,
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, ".*boom")

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieverWaitingForConnection(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	requested := make(chan struct{})
	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(context.Context, api.Connection) error) error {
		close(requested)

		select {
		case <-ctx.Done():
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out waiting for context to be done")
		}
		return nil
	})
	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	// If we're waiting for a connection, a cancel should stop the retriever.
	go func() {
		select {
		case <-requested:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out waiting for connection to be requested")
		}

		cancel()
	}()

	_, _, err := ret.Retrieve(ctx, "foo")
	c.Assert(err, tc.ErrorIs, context.Canceled)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) newRetriever(c *tc.C) *BlobRetriever {
	ret, err := NewBlobRetriever(s.remoteCallers, "namespace", func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
		return s.client, nil
	}, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	return ret
}

func (s *retrieverSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.remoteCallers = NewMockAPIRemoteCallers(ctrl)
	s.remoteConnection = NewMockRemoteConnection(ctrl)
	s.apiConnection = NewMockConnection(ctrl)

	s.client = NewMockBlobsClient(ctrl)
	s.clock = NewMockClock(ctrl)
	s.clock.EXPECT().Now().AnyTimes().Return(time.Now())

	return ctrl
}
