// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"errors"
	io "io"
	"runtime"
	"strings"
	"testing"
	"time"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	api "github.com/juju/juju/api"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testhelpers"
	apiremotecaller "github.com/juju/juju/internal/worker/apiremotecaller"
)

type retrieverSuite struct {
	testhelpers.IsolationSuite

	remoteCallers     *MockAPIRemoteCallers
	remoteConnection1 *MockRemoteConnection
	remoteConnection2 *MockRemoteConnection
	apiConnection     *MockConnection
	simpleHTTPClient  *MockSimpleHTTPClient
	client            *MockBlobsClient
	clock             *MockClock
}

func TestRetrieverSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &retrieverSuite{})
}

func (s *retrieverSuite) TestRetrieve(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection1}, nil)
	s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil)
	s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api")
	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveControllerNamespace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection1}, nil)
	s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().ModelTag().Return(names.NewModelTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"), true)
	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil)
	s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api")
	s.client.EXPECT().GetObject(gomock.Any(), "f47ac10b-58cc-4372-a567-0e02b2c3d479", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil)

	ret, err := NewBlobRetriever(s.remoteCallers, database.ControllerNS, func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
		return s.client, nil
	}, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveControllerNamespacePerCall(c *tc.C) {
	defer s.setupMocks(c).Finish()

	tag1 := names.NewModelTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	tag2 := names.NewModelTag("48c4a9b5-3861-497e-88bd-4d94f96931d5")

	gomock.InOrder(
		s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection1}, nil),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.apiConnection.EXPECT().ModelTag().Return(tag1, true),
		s.client.EXPECT().GetObject(gomock.Any(), tag1.Id(), "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),

		s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection1}, nil),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.apiConnection.EXPECT().ModelTag().Return(tag2, true),
		s.client.EXPECT().GetObject(gomock.Any(), tag2.Id(), "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	ret, err := NewBlobRetriever(s.remoteCallers, database.ControllerNS, func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
		return s.client, nil
	}, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	reader, size, err = ret.Retrieve(c.Context(), "sha256", []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveControllerNamespaceMissingModelTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection}, nil)
	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil)
	s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api")
	s.apiConnection.EXPECT().ModelTag().Return(names.NewModelTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"), false)

	ret, err := NewBlobRetriever(s.remoteCallers, database.ControllerNS, func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
		return s.client, nil
	}, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, ret)

	_, _, err = ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorMatches, ".*missing model tag.*")

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection1,
		s.remoteConnection1,
	}, nil)

	gomock.InOrder(
		s.remoteConnection1.EXPECT().ControllerID().Return("1"),
		s.remoteConnection1.EXPECT().ControllerID().Return("0"),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotesWithHints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection1,
		s.remoteConnection2,
	}, nil)

	gomock.InOrder(
		s.remoteConnection1.EXPECT().ControllerID().Return("1"),
		s.remoteConnection2.EXPECT().ControllerID().Return("0"),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.remoteConnection2.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotesWithHintsDifferentOrdering(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection1,
		s.remoteConnection2,
	}, nil)

	gomock.InOrder(
		s.remoteConnection1.EXPECT().ControllerID().Return("1"),
		s.remoteConnection2.EXPECT().ControllerID().Return("0"),
		s.remoteConnection2.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"0"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotesWithNoHints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection1,
		s.remoteConnection2,
	}, nil)

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil).Times(2)
	s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api").Times(2)

	s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.remoteConnection2.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotesWithDifferentHints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection1,
		s.remoteConnection2,
	}, nil)

	gomock.InOrder(
		s.remoteConnection1.EXPECT().ControllerID().Return("1"),
		s.remoteConnection2.EXPECT().ControllerID().Return("0"),
	)

	gomock.InOrder(
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil).Times(2)
	s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api").Times(2)

	s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.remoteConnection2.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", []string{"100", "200"})
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotesAllFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection1,
		s.remoteConnection1,
	}, nil)

	gomock.InOrder(
		s.remoteConnection1.EXPECT().ControllerID().Return("1"),
		s.remoteConnection1.EXPECT().ControllerID().Return("0"),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil),
		s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api"),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
	)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256", []string{"2", "3", "1"})
	c.Assert(err, tc.ErrorIs, BlobNotFound)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveRemotesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return(nil, jujuerrors.NotFound)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256", nil)
	c.Assert(err, tc.ErrorIs, jujuerrors.NotFound)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveRemotesNoRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return(nil, nil)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256", nil)
	c.Assert(err, tc.ErrorIs, NoRemoteConnections)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveNoHTTPClient(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection1}, nil)
	s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, errors.New("boom"))

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256", nil)
	c.Assert(err, tc.ErrorMatches, `.*boom.*`)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveCanceled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, _, err := ret.Retrieve(ctx, "sha256", nil)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *retrieverSuite) TestRetrieveDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	ret.Kill()

	_, _, err := ret.Retrieve(c.Context(), "sha256", nil)
	c.Assert(err, tc.ErrorIs, tomb.ErrDying)
}

// If the connection function context is canceled, and the reader takes that
// context, then the reader should not prevent the context from being
// canceled.
func (s *retrieverSuite) TestRetrievePreventReaderCancelationPropagate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sync := make(chan struct{})

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection1}, nil)
	s.remoteConnection1.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		ctx, cancel := context.WithCancel(ctx)
		defer func() {
			cancel()
			close(sync)
		}()
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().SimpleHTTPClient().Return(s.simpleHTTPClient, nil)
	s.simpleHTTPClient.EXPECT().BaseURL().Return("http://example.com/api")
	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").DoAndReturn(func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
		return newCancelableReader(ctx, strings.NewReader("test data")), int64(9), nil
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256", nil)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("context should not be done yet: %v", c.Context().Err())
	}

	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestScopedContextDoneClosesOnChildDone(c *tc.C) {
	parentCtx, parentCancel := context.WithCancel(c.Context())
	defer parentCancel()

	childCtx, childCancel := context.WithCancel(c.Context())
	defer childCancel()

	ctx := &scopedContext{
		parent: parentCtx,
		child:  childCtx,
	}

	done := ctx.Done()
	childCancel()

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("done channel did not close when child context was cancelled: %v", c.Context().Err())
	}
}

func (s *retrieverSuite) TestScopedContextDoneIgnoresChildAfterIgnore(c *tc.C) {
	parentCtx, parentCancel := context.WithCancel(c.Context())
	defer parentCancel()

	childCtx, childCancel := context.WithCancel(c.Context())
	defer childCancel()

	ctx := &scopedContext{
		parent: parentCtx,
		child:  childCtx,
	}

	done := ctx.Done()
	ctx.IgnoreChild()
	childCancel()

	// Give the goroutine behind Done a chance to process child cancellation.
	for i := 0; i < 1000; i++ {
		runtime.Gosched()
	}

	select {
	case <-done:
		c.Fatal("done channel closed while child context is ignored")
	default:
	}

	parentCancel()
	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("done channel did not close when parent context was cancelled: %v", c.Context().Err())
	}
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
	s.remoteConnection1 = NewMockRemoteConnection(ctrl)
	s.remoteConnection2 = NewMockRemoteConnection(ctrl)
	s.apiConnection = NewMockConnection(ctrl)
	s.simpleHTTPClient = NewMockSimpleHTTPClient(ctrl)

	s.client = NewMockBlobsClient(ctrl)
	s.clock = NewMockClock(ctrl)
	s.clock.EXPECT().Now().AnyTimes().Return(time.Now())

	c.Cleanup(func() {
		s.remoteCallers = nil
		s.remoteConnection1 = nil
		s.remoteConnection2 = nil
		s.apiConnection = nil
		s.simpleHTTPClient = nil
		s.client = nil
		s.clock = nil
	})

	return ctrl
}

func (s *retrieverSuite) checkReader(c *tc.C, reader io.ReadCloser, size int64) {
	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, int64(len(data)))
	c.Check(data, tc.DeepEquals, []byte("test data"))

	err = reader.Close()
	c.Assert(err, tc.ErrorIsNil)
}

type cancelableReader struct {
	reader io.Reader
	ctx    context.Context
}

func newCancelableReader(ctx context.Context, r io.Reader) io.ReadCloser {
	return &cancelableReader{
		reader: r,
		ctx:    ctx,
	}
}

func (r *cancelableReader) Read(p []byte) (n int, err error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	return r.reader.Read(p)
}

func (r *cancelableReader) Close() error {
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	default:
		return nil
	}
}
