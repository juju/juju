// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
)

type storageStatusSuite struct {
	now time.Time
}

func TestStorageStatusSuite(t *testing.T) {
	tc.Run(t, &storageStatusSuite{})
}

func (s *storageStatusSuite) SetUpTest(c *tc.C) {
	s.now = time.Now()
}

func (s *storageStatusSuite) TestEncodeFilesystemStatus(c *tc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.StorageFilesystemStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Pending,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypePending,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Error,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeError,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Attaching,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeAttaching,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Attached,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeAttached,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Detaching,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeDetaching,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Detached,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeDetached,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Destroying,
			},
			output: status.StatusInfo[status.StorageFilesystemStatusType]{
				Status: status.StorageFilesystemStatusTypeDestroying,
			},
		},
	}

	for i, test := range testCases {
		c.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			t.Logf("test %d: %v", i, test.input)
			output, err := encodeFilesystemStatus(test.input)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Check(t, output, tc.DeepEquals, test.output)
		})
	}
}

func (s *storageStatusSuite) TestEncodeVolumeStatus(c *tc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.StorageVolumeStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Pending,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypePending,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Error,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypeError,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Attaching,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypeAttaching,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Attached,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypeAttached,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Detaching,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypeDetaching,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Detached,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypeDetached,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Destroying,
			},
			output: status.StatusInfo[status.StorageVolumeStatusType]{
				Status: status.StorageVolumeStatusTypeDestroying,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeVolumeStatus(test.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(output, tc.DeepEquals, test.output)
	}
}
