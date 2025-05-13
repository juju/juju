// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

const deleteTestId = "tst12345"

type deleteImageSuite struct {
	BaseCloudImageMetadataSuite

	mockAPI *mockDeleteAPI

	deletedIds []string
}

var _ = tc.Suite(&deleteImageSuite{})

func (s *deleteImageSuite) SetUpTest(c *tc.C) {
	s.BaseCloudImageMetadataSuite.SetUpTest(c)

	s.deletedIds = []string{}
	s.mockAPI = &mockDeleteAPI{
		delete: func(imageId string) error {
			s.deletedIds = append(s.deletedIds, imageId)
			return nil
		},
		Stub: &testhelpers.Stub{},
	}
}

func (s *deleteImageSuite) TestDeleteImageMetadata(c *tc.C) {
	s.assertDeleteImageMetadata(c, deleteTestId)
}

func (s *deleteImageSuite) TestDeleteImageMetadataNoImageId(c *tc.C) {
	s.assertDeleteImageMetadataErr(c, "image ID must be supplied when deleting image metadata")
}

func (s *deleteImageSuite) TestDeleteImageMetadataManyImageIds(c *tc.C) {
	s.assertDeleteImageMetadataErr(c, "only one image ID can be supplied as an argument to this command", deleteTestId, deleteTestId)
}

func (s *deleteImageSuite) TestDeleteImageMetadataFailed(c *tc.C) {
	msg := "failed"
	s.mockAPI.delete = func(imageId string) error {
		return errors.New(msg)
	}
	s.assertDeleteImageMetadataErr(c, msg, deleteTestId)
	s.mockAPI.CheckCallNames(c, "Delete", "Close")
}

func (s *deleteImageSuite) runDeleteImageMetadata(c *tc.C, args ...string) error {
	tstDelete := &deleteImageMetadataCommand{}
	tstDelete.SetClientStore(jujuclienttesting.MinimalStore())
	tstDelete.newAPIFunc = func(ctx context.Context) (MetadataDeleteAPI, error) {
		return s.mockAPI, nil
	}
	deleteCmd := modelcmd.Wrap(tstDelete)

	_, err := cmdtesting.RunCommand(c, deleteCmd, args...)
	return err
}

func (s *deleteImageSuite) assertDeleteImageMetadata(c *tc.C, id string) {
	err := s.runDeleteImageMetadata(c, id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.deletedIds, tc.DeepEquals, []string{id})
	s.mockAPI.CheckCallNames(c, "Delete", "Close")
}

func (s *deleteImageSuite) assertDeleteImageMetadataErr(c *tc.C, errorMsg string, args ...string) {
	err := s.runDeleteImageMetadata(c, args...)
	c.Assert(err, tc.ErrorMatches, errorMsg)
	c.Assert(s.deletedIds, tc.DeepEquals, []string{})
}

type mockDeleteAPI struct {
	*testhelpers.Stub

	delete func(imageId string) error
}

func (s mockDeleteAPI) Close() error {
	s.MethodCall(s, "Close")
	return nil
}

func (s mockDeleteAPI) Delete(ctx context.Context, imageId string) error {
	s.MethodCall(s, "Delete", imageId)
	return s.delete(imageId)
}
