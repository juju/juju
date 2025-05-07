// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package download_test

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/internal/handlers/resources/download"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type ValidateSuite struct {
	testing.IsolationSuite

	fileSystem *MockFileSystem
}

var _ = tc.Suite(&ValidateSuite{})

func (s *ValidateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fileSystem = NewMockFileSystem(ctrl)

	return ctrl
}

func (s *ValidateSuite) TestValidateResource(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	resourceContent := []byte("resource blob content")
	hasher := sha512.New384()
	size, err := io.Copy(hasher, strings.NewReader(string(resourceContent)))
	c.Assert(err, tc.ErrorIsNil)
	hash := hex.EncodeToString(hasher.Sum(nil))

	downloader := download.NewDownloader(
		loggertesting.WrapCheckLog(c),
		s.fileSystem,
	)

	f, closer := s.expectTmpFile(c)
	defer closer()
	s.fileSystem.EXPECT().CreateTemp("", "resource-").Return(f, nil)
	s.fileSystem.EXPECT().Open(f.Name()).DoAndReturn(func(string) (*os.File, error) {
		opened, err := os.Open(f.Name())
		c.Assert(err, tc.ErrorIsNil)
		return opened, nil
	})
	s.fileSystem.EXPECT().Remove(f.Name())

	// Act:
	reader, err := downloader.Download(
		context.Background(),
		io.NopCloser(bytes.NewBuffer(resourceContent)),
		hash,
		size,
	)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(buf.Bytes(), tc.DeepEquals, resourceContent)
	c.Assert(reader.Close(), tc.ErrorIsNil)
}

func (s *ValidateSuite) TestGetResourceUnexpectedSize(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	resourceContent := []byte("resource blob content")
	hasher := sha512.New384()
	_, err := io.Copy(hasher, strings.NewReader(string(resourceContent)))
	c.Assert(err, tc.ErrorIsNil)
	hash := hex.EncodeToString(hasher.Sum(nil))

	downloader := download.NewDownloader(
		loggertesting.WrapCheckLog(c),
		s.fileSystem,
	)

	f, closer := s.expectTmpFile(c)
	defer closer()
	s.fileSystem.EXPECT().CreateTemp("", "resource-").Return(f, nil)
	s.fileSystem.EXPECT().Remove(f.Name())

	// Act:
	reader, err := downloader.Download(
		context.Background(),
		io.NopCloser(bytes.NewBuffer(resourceContent)),
		hash,
		666,
	)

	// Assert:
	c.Assert(err, tc.ErrorMatches, "downloaded resource has unexpected size.*")
	c.Assert(reader, tc.IsNil)
}

func (s *ValidateSuite) TestGetResourceUnexpectedHash(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	resourceContent := []byte("resource blob content")
	size := int64(len(resourceContent))

	downloader := download.NewDownloader(
		loggertesting.WrapCheckLog(c),
		s.fileSystem,
	)

	f, closer := s.expectTmpFile(c)
	defer closer()
	s.fileSystem.EXPECT().CreateTemp("", "resource-").Return(f, nil)
	s.fileSystem.EXPECT().Remove(f.Name())

	// Act:
	reader, err := downloader.Download(
		context.Background(),
		io.NopCloser(bytes.NewBuffer(resourceContent)),
		"bad-hash",
		size,
	)

	// Assert:
	c.Assert(err, tc.ErrorMatches, "downloaded resource has unexpected hash.*")
	c.Assert(reader, tc.IsNil)
}

func (s *ValidateSuite) expectTmpFile(c *tc.C) (*os.File, func()) {
	tmpFile, err := os.CreateTemp("", "resource-")
	c.Assert(err, tc.ErrorIsNil)

	return tmpFile, func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, tc.ErrorIsNil)
	}
}
