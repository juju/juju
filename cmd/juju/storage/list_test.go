// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"errors"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
)

type ListSuite struct {
	SubStorageSuite
	mockAPI *mockListAPI
}

var _ = gc.Suite(&ListSuite{})

func (s *ListSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockListAPI{}
}

func (s *ListSuite) runList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, storage.NewListCommandForTest(s.mockAPI, s.store), args...)
}

func (s *ListSuite) TestList(c *gc.C) {
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
\[Storage\]
Unit          Id           Type        Pool      Provider id                 Size    Status    Message
postgresql/0  db-dir/1100  block                                                     attached  
transcode/0   db-dir/1000  block                                                     pending   creating volume
transcode/0   shared-fs/0  filesystem  radiance  provider-supplied-volume-4  1.0GiB  attached  
transcode/1   shared-fs/0  filesystem  radiance  provider-supplied-volume-4  1.0GiB  attached  

`[1:])
}

func (s *ListSuite) TestListNoPool(c *gc.C) {
	s.mockAPI.omitPool = true
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
\[Storage\]
Unit          Id           Type        Provider id                 Size    Status    Message
postgresql/0  db-dir/1100  block                                           attached  
transcode/0   db-dir/1000  block                                           pending   creating volume
transcode/0   shared-fs/0  filesystem  provider-supplied-volume-4  1.0GiB  attached  
transcode/1   shared-fs/0  filesystem  provider-supplied-volume-4  1.0GiB  attached  

`[1:])
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
storage:
  db-dir/1000:
    kind: block
    status:
      current: pending
      message: creating volume
      since: .*
    persistent: false
    attachments:
      units:
        transcode/0:
          location: thither
  db-dir/1100:
    kind: block
    life: dying
    status:
      current: attached
      since: .*
    persistent: true
    attachments:
      units:
        postgresql/0:
          location: hither
          life: dying
  shared-fs/0:
    kind: filesystem
    status:
      current: attached
      since: .*
    persistent: true
    attachments:
      units:
        transcode/0:
          location: there
        transcode/1:
          location: here
filesystems:
  0/0:
    provider-id: provider-supplied-filesystem-0-0
    volume: 0/1
    storage: db-dir/1001
    attachments:
      machines:
        "0":
          mount-point: /mnt/fuji
          read-only: false
          life: alive
      units:
        abc/0:
          machine: "0"
          location: /mnt/fuji
    size: 512
    life: alive
    status:
      current: attached
      since: .*
  "1":
    provider-id: provider-supplied-filesystem-1
    attachments:
      machines:
        "0":
          mount-point: ""
          read-only: false
    size: 2048
    status:
      current: attaching
      message: failed to attach, will retry
      since: .*
  "2":
    provider-id: provider-supplied-filesystem-2
    attachments:
      machines:
        "1":
          mount-point: /mnt/zion
          read-only: false
    size: 3
    status:
      current: attached
      since: .*
  "3":
    attachments:
      machines:
        "1":
          mount-point: ""
          read-only: false
    size: 42
    status:
      current: pending
      since: .*
  "4":
    provider-id: provider-supplied-filesystem-4
    storage: shared-fs/0
    attachments:
      machines:
        "0":
          mount-point: /mnt/doom
          read-only: true
        "1":
          mount-point: /mnt/huang
          read-only: true
      units:
        transcode/0:
          machine: "0"
          location: /mnt/bits
        transcode/1:
          machine: "1"
          location: /mnt/pieces
    pool: radiance
    size: 1024
    status:
      current: attached
      since: .*
volumes:
  0/0:
    provider-id: provider-supplied-volume-0-0
    storage: db-dir/1001
    attachments:
      machines:
        "0":
          device: loop0
          read-only: false
          life: alive
      units:
        abc/0:
          machine: "0"
          location: /dev/loop0
    pool: radiance
    size: 512
    persistent: false
    life: alive
    status:
      current: attached
      since: .*
  "1":
    provider-id: provider-supplied-volume-1
    attachments:
      machines:
        "0":
          read-only: false
    hardware-id: serial blah blah
    size: 2048
    persistent: true
    status:
      current: attaching
      message: failed to attach, will retry
      since: .*
  "2":
    provider-id: provider-supplied-volume-2
    attachments:
      machines:
        "1":
          device: xvdf1
          read-only: false
    size: 3
    persistent: false
    status:
      current: attached
      since: .*
  "3":
    attachments:
      machines:
        "1":
          read-only: false
    size: 42
    persistent: false
    status:
      current: pending
      since: .*
  "4":
    provider-id: provider-supplied-volume-4
    storage: shared-fs/0
    attachments:
      machines:
        "0":
          device: xvdf2
          read-only: true
        "1":
          device: xvdf3
          read-only: true
      units:
        transcode/0:
          machine: "0"
          location: /mnt/bits
        transcode/1:
          machine: "1"
          location: /mnt/pieces
    size: 1024
    persistent: true
    status:
      current: attached
      since: .*
`[1:])
}

func (s *ListSuite) TestListInitErrors(c *gc.C) {
	s.testListInitError(c, []string{"--filesystem", "--volume"}, "--filesystem and --volume can not be used together")
	s.testListInitError(c, []string{"storage-id"}, "specifying IDs only supported with --filesystem and --volume flags")
}

func (s *ListSuite) testListInitError(c *gc.C, args []string, expectedErr string) {
	_, err := s.runList(c, args)
	c.Assert(err, gc.ErrorMatches, expectedErr)
}

func (s *ListSuite) TestListError(c *gc.C) {
	s.mockAPI.listErrors = true
	context, err := s.runList(c, nil)
	c.Assert(err, gc.ErrorMatches, "list fails")
	stderr := testing.Stderr(context)
	c.Assert(stderr, gc.Equals, "")
	stdout := testing.Stdout(context)
	c.Assert(stdout, gc.Equals, "")
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expectedValid string) {
	context, err := s.runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, "")

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Matches, expectedValid)
}

type mockListAPI struct {
	listErrors      bool
	listFilesystems func([]string) ([]params.FilesystemDetailsListResult, error)
	listVolumes     func([]string) ([]params.VolumeDetailsListResult, error)
	omitPool        bool
}

func (s *mockListAPI) Close() error {
	return nil
}

func (s *mockListAPI) ListStorageDetails() ([]params.StorageDetails, error) {
	if s.listErrors {
		return nil, errors.New("list fails")
	}

	// postgresql/0 has "db-dir/1100"
	// transcode/1 has "db-dir/1000"
	// transcode/0 and transcode/1 share "shared-fs/0"
	//
	// there is also a storage instance "db-dir/1010" which
	// returns an error when listed.
	results := []params.StorageDetails{{
		StorageTag: "storage-db-dir-1000",
		OwnerTag:   "unit-transcode-0",
		Kind:       params.StorageKindBlock,
		Status: params.EntityStatus{
			Status: status.Pending,
			Since:  &epoch,
			Info:   "creating volume",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-transcode-0": params.StorageAttachmentDetails{
				Location: "thither",
			},
		},
	}, {
		StorageTag: "storage-db-dir-1100",
		OwnerTag:   "unit-postgresql-0",
		Kind:       params.StorageKindBlock,
		Life:       "dying",
		Status: params.EntityStatus{
			Status: status.Attached,
			Since:  &epoch,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-postgresql-0": params.StorageAttachmentDetails{
				Location: "hither",
				Life:     "dying",
			},
		},
	}, {
		StorageTag: "storage-shared-fs-0",
		OwnerTag:   "application-transcode",
		Kind:       params.StorageKindFilesystem,
		Status: params.EntityStatus{
			Status: status.Attached,
			Since:  &epoch,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-transcode-0": params.StorageAttachmentDetails{
				Location: "there",
			},
			"unit-transcode-1": params.StorageAttachmentDetails{
				Location: "here",
			},
		},
	}}
	return results, nil
}
