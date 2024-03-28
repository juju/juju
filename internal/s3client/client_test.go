// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type s3ClientSuite struct {
	s3Client *MockS3Client
}

var _ = gc.Suite(&s3ClientSuite{})

func (s *s3ClientSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.s3Client = NewMockS3Client(ctrl)

	return ctrl
}

func (s *s3ClientSuite) TestGetObject(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.s3Client.EXPECT().GetObject(gomock.Any(), &s3.GetObjectInput{
		Bucket: aws.String("bucket"),
		Key:    aws.String("object"),
	}, gomock.Any()).Return(&s3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader("blob")),
	}, nil)

	cli := objectsClient{
		client: s.s3Client,
		logger: loggo.GetLogger("juju.testing.s3client"),
	}
	resp, err := cli.GetObject(context.Background(), "bucket", "object")
	c.Assert(err, jc.ErrorIsNil)

	blob, err := io.ReadAll(resp)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(blob), gc.Equals, "blob")
}
