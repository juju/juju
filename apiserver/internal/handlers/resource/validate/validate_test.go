// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package validate_test

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/internal/handlers/resource/validate"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type ValidateSuite struct {
	testing.IsolationSuite

	fileSystem *MockFileSystem
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.fileSystem = NewMockFileSystem(ctrl)

	return ctrl
}

func (s *ValidateSuite) TestValidateResource(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	resourceContent := []byte("resource blob content")
	hasher := sha512.New384()
	size, err := io.Copy(hasher, strings.NewReader(string(resourceContent)))
	c.Assert(err, jc.ErrorIsNil)
	hash := hex.EncodeToString(hasher.Sum(nil))

	validator := validate.NewValidator(
		loggertesting.WrapCheckLog(c),
		s.fileSystem,
	)

	f, closer := s.expectTmpFile(c)
	defer closer()
	s.fileSystem.EXPECT().CreateTemp("", "resource-").Return(f, nil)
	s.fileSystem.EXPECT().Open(f.Name()).DoAndReturn(func(string) (*os.File, error) {
		opened, err := os.Open(f.Name())
		c.Assert(err, jc.ErrorIsNil)
		return opened, nil
	})
	s.fileSystem.EXPECT().Remove(f.Name())

	// Act:
	reader, err := validator.Validate(
		io.NopCloser(bytes.NewBuffer(resourceContent)),
		hash,
		size,
	)

	// Assert:
	c.Assert(err, jc.ErrorIsNil)
	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.Bytes(), gc.DeepEquals, resourceContent)
	c.Assert(reader.Close(), jc.ErrorIsNil)
}

func (s *ValidateSuite) TestGetResourceUnexpectedSize(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	resourceContent := []byte("resource blob content")
	hasher := sha512.New384()
	_, err := io.Copy(hasher, strings.NewReader(string(resourceContent)))
	c.Assert(err, jc.ErrorIsNil)
	hash := hex.EncodeToString(hasher.Sum(nil))

	validator := validate.NewValidator(
		loggertesting.WrapCheckLog(c),
		s.fileSystem,
	)

	f, closer := s.expectTmpFile(c)
	defer closer()
	s.fileSystem.EXPECT().CreateTemp("", "resource-").Return(f, nil)
	s.fileSystem.EXPECT().Remove(f.Name())

	// Act:
	reader, err := validator.Validate(
		io.NopCloser(bytes.NewBuffer(resourceContent)),
		hash,
		666,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, validate.ErrUnexpectedSize)
	c.Assert(reader, gc.IsNil)
}

func (s *ValidateSuite) TestGetResourceUnexpectedHash(c *gc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:

	resourceContent := []byte("resource blob content")
	size := int64(len(resourceContent))

	validator := validate.NewValidator(
		loggertesting.WrapCheckLog(c),
		s.fileSystem,
	)

	f, closer := s.expectTmpFile(c)
	defer closer()
	s.fileSystem.EXPECT().CreateTemp("", "resource-").Return(f, nil)
	s.fileSystem.EXPECT().Remove(f.Name())

	// Act:
	reader, err := validator.Validate(
		io.NopCloser(bytes.NewBuffer(resourceContent)),
		"bad-hash",
		size,
	)

	// Assert:
	c.Assert(err, jc.ErrorIs, validate.ErrUnexpectedHash)
	c.Assert(reader, gc.IsNil)
}

func (s *ValidateSuite) expectTmpFile(c *gc.C) (*os.File, func()) {
	tmpFile, err := os.CreateTemp("", "resource-")
	c.Assert(err, jc.ErrorIsNil)

	return tmpFile, func() {
		err := os.Remove(tmpFile.Name())
		c.Assert(err, jc.ErrorIsNil)
	}
}
