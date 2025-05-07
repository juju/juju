// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination state_mock_test.go github.com/juju/juju/internal/objectstore Claimer,ClaimExtender,HashFileSystemAccessor,TrackedObjectStore,RemoteRetriever
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStoreMetadata,Session
//go:generate go run go.uber.org/mock/mockgen -typed -package objectstore -destination clock_mock_test.go github.com/juju/clock Clock

func TestAll(t *stdtesting.T) {
	defer goleak.VerifyNone(t)

	tc.TestingT(t)
}

type baseSuite struct {
	testhelpers.IsolationSuite

	service       *MockObjectStoreMetadata
	claimer       *MockClaimer
	claimExtender *MockClaimExtender
}

func (s *baseSuite) setupMocks(c *tc.C) *gomock.Controller {
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

func (s *baseSuite) readFile(c *tc.C, reader io.ReadCloser) string {
	defer reader.Close()

	content, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	return string(content)
}

func (s *baseSuite) calculateHexSHA384(c *tc.C, contents string) string {
	hasher := sha512.New384()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, tc.ErrorIsNil)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (s *baseSuite) calculateHexSHA256(c *tc.C, contents string) string {
	hasher := sha256.New()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, tc.ErrorIsNil)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (s *baseSuite) calculateBase64SHA256(c *tc.C, contents string) string {
	hasher := sha256.New()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, tc.ErrorIsNil)
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}

func (s *baseSuite) createFile(c *tc.C, path, name, contents string) (int64, string, string) {
	// Ensure the directory exists.
	err := os.MkdirAll(path, 0755)
	c.Assert(err, tc.ErrorIsNil)

	// Create a file in a temporary directory.
	dir := c.MkDir()

	f, err := os.Create(filepath.Join(dir, name))
	c.Assert(err, tc.ErrorIsNil)
	defer f.Close()

	// Create a hash of the contents when writing the file. The hash will
	// be used as the file name on disk.
	hasher384 := sha512.New384()
	hasher256 := sha256.New()

	size, err := io.Copy(f, io.TeeReader(strings.NewReader(contents), io.MultiWriter(hasher384, hasher256)))
	c.Assert(err, tc.ErrorIsNil)

	info, err := f.Stat()
	c.Assert(err, tc.ErrorIsNil)

	if info.Size() != size {
		c.Fatalf("file size %d does not match expected size %d", info.Size(), size)
	}

	hash384 := hex.EncodeToString(hasher384.Sum(nil))
	err = os.Rename(filepath.Join(dir, name), filepath.Join(path, hash384))
	c.Assert(err, tc.ErrorIsNil)

	return info.Size(), hash384, hex.EncodeToString(hasher256.Sum(nil))
}
