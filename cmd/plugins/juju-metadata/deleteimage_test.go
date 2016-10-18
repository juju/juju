// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/testing"
)

const deleteTestId = "tst12345"

type deleteImageSuite struct {
	BaseCloudImageMetadataSuite

	mockAPI *mockDeleteAPI

	deletedIds []string
}

var _ = gc.Suite(&deleteImageSuite{})

func (s *deleteImageSuite) SetUpTest(c *gc.C) {
	s.BaseCloudImageMetadataSuite.SetUpTest(c)

	s.deletedIds = []string{}
	s.mockAPI = &mockDeleteAPI{
		delete: func(imageId string) error {
			s.deletedIds = append(s.deletedIds, imageId)
			return nil
		},
		Stub: &gitjujutesting.Stub{},
	}
}

func (s *deleteImageSuite) TestDeleteImageMetadata(c *gc.C) {
	s.assertDeleteImageMetadata(c, deleteTestId)
}

func (s *deleteImageSuite) TestDeleteImageMetadataNoImageId(c *gc.C) {
	s.assertDeleteImageMetadataErr(c, "image ID must be supplied when deleting image metadata")
}

func (s *deleteImageSuite) TestDeleteImageMetadataManyImageIds(c *gc.C) {
	s.assertDeleteImageMetadataErr(c, "only one image ID can be supplied as an argument to this command", deleteTestId, deleteTestId)
}

func (s *deleteImageSuite) TestDeleteImageMetadataFailed(c *gc.C) {
	msg := "failed"
	s.mockAPI.delete = func(imageId string) error {
		return errors.New(msg)
	}
	s.assertDeleteImageMetadataErr(c, msg, deleteTestId)
	s.mockAPI.CheckCallNames(c, "Delete", "Close")
}

func (s *deleteImageSuite) runDeleteImageMetadata(c *gc.C, args ...string) error {
	tstDelete := &deleteImageMetadataCommand{}
	tstDelete.newAPIFunc = func() (MetadataDeleteAPI, error) {
		return s.mockAPI, nil
	}
	deleteCmd := modelcmd.Wrap(tstDelete)

	_, err := testing.RunCommand(c, deleteCmd, args...)
	return err
}

func (s *deleteImageSuite) assertDeleteImageMetadata(c *gc.C, id string) {
	err := s.runDeleteImageMetadata(c, id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.deletedIds, gc.DeepEquals, []string{id})
	s.mockAPI.CheckCallNames(c, "Delete", "Close")
}

func (s *deleteImageSuite) assertDeleteImageMetadataErr(c *gc.C, errorMsg string, args ...string) {
	err := s.runDeleteImageMetadata(c, args...)
	c.Assert(err, gc.ErrorMatches, errorMsg)
	c.Assert(s.deletedIds, gc.DeepEquals, []string{})
}

type mockDeleteAPI struct {
	*gitjujutesting.Stub

	delete func(imageId string) error
}

func (s mockDeleteAPI) Close() error {
	s.MethodCall(s, "Close")
	return nil
}

func (s mockDeleteAPI) Delete(imageId string) error {
	s.MethodCall(s, "Delete", imageId)
	return s.delete(imageId)
}
