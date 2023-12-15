// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	context "context"
	"errors"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	httprequest "gopkg.in/httprequest.v1"
)

type clientSuite struct {
	baseSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestNoAPIAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().Addr().Return("")

	sess, err := NewS3Client(s.apiConn, s.logger)
	c.Check(sess, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "API address not available for S3 client")
}

func (s *clientSuite) TestNoHTTPClient(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().Addr().Return("current-addr")
	s.apiConn.EXPECT().HTTPClient().Return(nil, errors.New("boom"))

	sess, err := NewS3Client(s.apiConn, s.logger)
	c.Check(sess, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot retrieve http client from the api connection: boom")
}

func (s *clientSuite) TestAWSClientFromConfigEmptyHTTPClient(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.apiConn.EXPECT().Addr().Return("current-addr")
	s.apiConn.EXPECT().HTTPClient().Return(&httprequest.Client{}, nil)

	sess, err := NewS3Client(s.apiConn, s.logger)
	c.Assert(sess, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)

	_, err = sess.GetObject(context.Background(), "bucket-name", "object-name")
	c.Assert(err, gc.ErrorMatches, "unable to get object object-name on bucket bucket-name using S3 client.*")
}
