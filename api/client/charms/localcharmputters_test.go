// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"io"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type fallbackPutterSuite struct {
	putters        []*MockCharmPutter
	fallbackPutter CharmPutter
}

var _ = gc.Suite(&fallbackPutterSuite{})

func (s *fallbackPutterSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.putters = []*MockCharmPutter{NewMockCharmPutter(ctrl), NewMockCharmPutter(ctrl), NewMockCharmPutter(ctrl)}

	var err error
	s.fallbackPutter, err = newFallbackPutter(s.putters[0], s.putters[1], s.putters[2])
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

const (
	charmBlob = "charm blob"
	charmRef  = "dummy-abcedf0"
	curlReq   = "local:jammy/dummy-0"
	curlResp  = "local:focal/dummy-1"
)

func (s *fallbackPutterSuite) TestFirstSucceeds(c *gc.C) {
	defer s.setup(c).Finish()

	s.putters[0].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).Return(curlResp, nil)

	// NOTE strings.Reader is not a closer, so here we also implicitly test that case
	body := strings.NewReader(charmBlob)
	resp, err := s.fallbackPutter.PutCharm(context.Background(), testing.ModelTag.Id(), charmRef, curlReq, body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp, gc.Equals, curlResp)
}

func (s *fallbackPutterSuite) TestFallbackableError(c *gc.C) {
	defer s.setup(c).Finish()

	s.putters[0].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).Return("", errors.NotFoundf("ep not found"))
	s.putters[1].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).Return(curlResp, nil)

	body := strings.NewReader(charmBlob)
	resp, err := s.fallbackPutter.PutCharm(context.Background(), testing.ModelTag.Id(), charmRef, curlReq, body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp, gc.Equals, curlResp)
}

func (s *fallbackPutterSuite) TestDoubleFallbackableError(c *gc.C) {
	defer s.setup(c).Finish()

	s.putters[0].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).Return("", errors.NotFoundf("ep not found"))
	s.putters[1].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).Return("", errors.MethodNotAllowedf("bad method"))
	s.putters[2].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).Return(curlResp, nil)

	body := strings.NewReader(charmBlob)
	resp, err := s.fallbackPutter.PutCharm(context.Background(), testing.ModelTag.Id(), charmRef, curlReq, body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp, gc.Equals, curlResp)
}

// TestCloseCloserOnce ensures that the stream we pass to the putter is only closed once,
// at then end of execution. This is because httpclient.Do (and as a result PutCharm) will
// often try to close bodies passed to them, which will break the fallback mechanism.
// So assert that, even when PutObject closes the stream it's passed, thos does not close
// the underlying stream.
func (s *fallbackPutterSuite) TestCloseCloserOnce(c *gc.C) {
	defer s.setup(c).Finish()

	body := &streamShim{stream: strings.NewReader(charmBlob)}

	s.putters[0].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ string, reader io.Reader) (string, error) {
			reader.(io.Closer).Close()
			c.Assert(body.closed, jc.IsFalse)
			return "", errors.NotFoundf("ep not found")
		},
	)
	s.putters[1].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ string, reader io.Reader) (string, error) {
			reader.(io.Closer).Close()
			c.Assert(body.closed, jc.IsFalse)
			return "", errors.MethodNotAllowedf("bad method")
		},
	)
	s.putters[2].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ string, reader io.Reader) (string, error) {
			reader.(io.Closer).Close()
			c.Assert(body.closed, jc.IsFalse)
			return curlResp, nil
		},
	)

	resp, err := s.fallbackPutter.PutCharm(context.Background(), testing.ModelTag.Id(), charmRef, curlReq, body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(body.closed, jc.IsTrue)
	c.Assert(resp, gc.Equals, curlResp)
}

func (s *fallbackPutterSuite) TestBodyReset(c *gc.C) {
	defer s.setup(c).Finish()

	body := strings.NewReader(charmBlob)

	s.putters[0].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ string, reader io.Reader) (string, error) {
			blob, err := io.ReadAll(reader)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(blob), gc.Equals, charmBlob)
			return "", errors.NotFoundf("ep not found")
		},
	)
	s.putters[1].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ string, reader io.Reader) (string, error) {
			blob, err := io.ReadAll(reader)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(blob), gc.Equals, charmBlob)
			return "", errors.MethodNotAllowedf("bad method")
		},
	)
	s.putters[2].EXPECT().PutCharm(gomock.Any(), testing.ModelTag.Id(), charmRef, curlReq, gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _, _ string, reader io.Reader) (string, error) {
			blob, err := io.ReadAll(reader)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(string(blob), gc.Equals, charmBlob)
			return curlResp, nil
		},
	)

	resp, err := s.fallbackPutter.PutCharm(context.Background(), testing.ModelTag.Id(), charmRef, curlReq, body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp, gc.Equals, curlResp)
}

type streamShim struct {
	closed bool
	stream io.ReadSeeker
}

func (s *streamShim) Read(p []byte) (n int, err error) {
	return s.stream.Read(p)
}

func (s *streamShim) Seek(offset int64, whence int) (int64, error) {
	return s.stream.Seek(offset, whence)
}

func (s *streamShim) Close() error {
	s.closed = true
	return nil
}
