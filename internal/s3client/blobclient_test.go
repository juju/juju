// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"bytes"
	"context"
	"io"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coretesting "github.com/juju/juju/internal/testing"
)

type charmsS3ClientSuite struct {
	session *MockSession
}

var _ = tc.Suite(&charmsS3ClientSuite{})

func (s *charmsS3ClientSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.session = NewMockSession(ctrl)

	return ctrl
}

func (s *charmsS3ClientSuite) TestGetCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().GetObject(gomock.Any(), "model-"+coretesting.ModelTag.Id(), "charms/somecharm-abcd0123").Return(io.NopCloser(bytes.NewBufferString("blob")), int64(4), "ignored", nil)

	cli := NewBlobsS3Client(s.session)
	body, err := cli.GetCharm(context.Background(), coretesting.ModelTag.Id(), "somecharm-abcd0123")
	c.Assert(err, tc.ErrorIsNil)

	bytes, err := io.ReadAll(body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(bytes, tc.DeepEquals, []byte("blob"))
}

func (s *charmsS3ClientSuite) TestGetObject(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash := "88e3f744a7555336193bff57b7d46c35a484dfbe8ef1dc977628c1d85a4ceaa5"

	s.session.EXPECT().GetObject(gomock.Any(), "model-"+coretesting.ModelTag.Id(), "objects/"+hash).Return(io.NopCloser(bytes.NewBufferString("blob")), int64(4), "ignored", nil)

	cli := NewBlobsS3Client(s.session)
	body, _, err := cli.GetObject(context.Background(), coretesting.ModelTag.Id(), hash)
	c.Assert(err, tc.ErrorIsNil)

	bytes, err := io.ReadAll(body)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(bytes, tc.DeepEquals, []byte("blob"))
}
