// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination state_mock_test.go github.com/juju/juju/internal/objectstore MongoSession,Claimer,ClaimExtender
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStoreMetadata,Session
//go:generate go run go.uber.org/mock/mockgen -package objectstore -destination clock_mock_test.go github.com/juju/clock Clock

func TestAll(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	gc.TestingT(t)
}

type baseSuite struct {
	testing.IsolationSuite

	service       *MockObjectStoreMetadata
	claimer       *MockClaimer
	claimExtender *MockClaimExtender
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockObjectStoreMetadata(ctrl)
	s.claimer = NewMockClaimer(ctrl)
	s.claimExtender = NewMockClaimExtender(ctrl)

	return ctrl
}

func (s *baseSuite) expectClaim(hash string, num int) {
	s.claimer.EXPECT().Claim(gomock.Any(), hash).Return(s.claimExtender, nil).Times(num)
	s.claimExtender.EXPECT().Extend(gomock.Any()).Return(nil).AnyTimes()
	s.claimExtender.EXPECT().Duration().Return(time.Second).AnyTimes()
}

func (s *baseSuite) expectRelease(hash string, num int) {
	s.claimer.EXPECT().Release(gomock.Any(), hash).Return(nil).Times(num)
}

func (s *baseSuite) readFile(c *gc.C, reader io.ReadCloser) string {
	defer reader.Close()

	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	return string(content)
}

func (s *baseSuite) calculateHexHash(c *gc.C, contents string) string {
	hasher := sha256.New()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, jc.ErrorIsNil)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (s *baseSuite) calculateBase64Hash(c *gc.C, contents string) string {
	hasher := sha256.New()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, jc.ErrorIsNil)
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}
