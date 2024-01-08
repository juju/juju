// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"

	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type charmsS3ClientSuite struct {
	session *MockSession
}

var _ = gc.Suite(&charmsS3ClientSuite{})

func (s *charmsS3ClientSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.session = NewMockSession(ctrl)

	return ctrl
}

func (s *charmsS3ClientSuite) TestGetCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().GetObject(gomock.Any(), "model-"+coretesting.ModelTag.Id(), "charms/somecharm-abcd0123")

	cli := NewCharmsS3Client(s.session)
	cli.GetCharm(context.Background(), coretesting.ModelTag.Id(), "somecharm-abcd0123")
}
