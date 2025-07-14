// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"context"
	"errors"
	io "io"
	"strings"
	"testing"
	"time"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/httprequest.v1"
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

func (s *retrieverSuite) TestRetrieve(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection}, nil)
	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)
	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveControllerNamespace(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection}, nil)
	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().ModelTag().Return(names.NewModelTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"), true)
	s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)
	s.client.EXPECT().GetObject(gomock.Any(), "f47ac10b-58cc-4372-a567-0e02b2c3d479", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil)

	ret, err := NewBlobRetriever(s.remoteCallers, database.ControllerNS, func(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
		return s.client, nil
	}, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
	}, nil)

	gomock.InOrder(
		s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(io.NopCloser(strings.NewReader("test data")), int64(9), nil),
	)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIsNil)
	s.checkReader(c, reader, size)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveMultipleRemotesAllFailed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{
		s.remoteConnection,
		s.remoteConnection,
	}, nil)

	gomock.InOrder(
		s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
		s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
			return f(ctx, s.apiConnection)
		}),
		s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil),
		s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").Return(nil, int64(-1), jujuerrors.NotFoundf("not found")),
	)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIs, BlobNotFound)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveRemotesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return(nil, jujuerrors.NotFound)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIs, jujuerrors.NotFound)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveRemotesNoRemotes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return(nil, nil)

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIs, NoRemoteConnections)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveNoHTTPClient(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection}, nil)
	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, errors.New("boom"))

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	_, _, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorMatches, `.*boom.*`)

	workertest.CleanKill(c, ret)
}

func (s *retrieverSuite) TestRetrieveCanceled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	_, _, err := ret.Retrieve(ctx, "sha256")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *retrieverSuite) TestRetrieveDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	ret.Kill()

	_, _, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIs, tomb.ErrDying)
}

// If the connection function context is canceled, and the reader takes that
// context, then the reader should not prevent the context from being
// canceled.
func (s *retrieverSuite) TestRetrievePreventReaderCancelationPropagate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sync := make(chan struct{})

	s.remoteCallers.EXPECT().GetAPIRemotes().Return([]apiremotecaller.RemoteConnection{s.remoteConnection}, nil)
	s.remoteConnection.EXPECT().Connection(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, f func(context.Context, api.Connection) error) error {
		ctx, cancel := context.WithCancel(ctx)
		defer func() {
			cancel()
			close(sync)
		}()
		return f(ctx, s.apiConnection)
	})
	s.apiConnection.EXPECT().RootHTTPClient().Return(&httprequest.Client{}, nil)
	s.client.EXPECT().GetObject(gomock.Any(), "namespace", "sha256").DoAndReturn(func(ctx context.Context, namespace, sha256 string) (io.ReadCloser, int64, error) {
		return newCancelableReader(ctx, strings.NewReader("test data")), int64(9), nil
	})

	ret := s.newRetriever(c)
	defer workertest.DirtyKill(c, ret)

	reader, size, err := ret.Retrieve(c.Context(), "sha256")
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("context should not be done yet: %v", c.Context().Err())
	}

	s.checkReader(c, reader, size)

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
