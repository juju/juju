// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"errors"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/rpc/params"
)

type ListSuite struct {
	SubStorageSuite
	mockAPI *mockListAPI
}

func TestListSuite(t *stdtesting.T) { tc.Run(t, &ListSuite{}) }
func (s *ListSuite) SetUpTest(c *tc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockListAPI{}
}

func (s *ListSuite) runList(c *tc.C, args []string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewListCommandForTest(s.mockAPI, s.store), args...)
}

func (s *ListSuite) TestList(c *tc.C) {
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
Unit          Storage ID    Type        Pool      Size     Status    Message
              persistent/1  filesystem                     detached  
postgresql/0  db-dir/1100   block                 3.0 MiB  attached  
transcode/0   db-dir/1000   block                          pending   creating volume
transcode/0   shared-fs/0   filesystem  radiance  1.0 GiB  attached  
transcode/1   shared-fs/0   filesystem  radiance  1.0 GiB  attached  
`[1:])
}

func (s *ListSuite) TestListNoPool(c *tc.C) {
	s.mockAPI.omitPool = true
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
Unit          Storage ID    Type        Size     Status    Message
              persistent/1  filesystem           detached  
postgresql/0  db-dir/1100   block       3.0 MiB  attached  
transcode/0   db-dir/1000   block                pending   creating volume
transcode/0   shared-fs/0   filesystem  1.0 GiB  attached  
transcode/1   shared-fs/0   filesystem  1.0 GiB  attached  
`[1:])
}

func (s *ListSuite) TestListYAML(c *tc.C) {
	now := time.Now()
	s.mockAPI.time = now
	since := common.FormatTime(&now, false)

	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		fmt.Sprintf(`
storage:
  db-dir/1000:
    kind: block
    status:
      current: pending
      message: creating volume
      since: %s
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
      since: %s
    persistent: true
    attachments:
      units:
        postgresql/0:
          location: hither
          life: dying
  persistent/1:
    kind: filesystem
    status:
      current: detached
      since: %s
    persistent: true
  shared-fs/0:
    kind: filesystem
    status:
      current: attached
      since: %s
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
      since: %s
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
      since: %s
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
      since: %s
  "3":
    attachments:
      machines:
        "1":
          mount-point: ""
          read-only: false
    size: 42
    status:
      current: pending
      since: %s
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
      since: %s
  "5":
    provider-id: provider-supplied-filesystem-5
    storage: db-dir/1100
    attachments:
      units:
        abc/0:
          location: /mnt/fuji
    size: 3
    status:
      current: attached
      since: %s
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
      since: %s
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
      since: %s
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
      since: %s
  "3":
    attachments:
      machines:
        "1":
          read-only: false
    size: 42
    persistent: false
    status:
      current: pending
      since: %s
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
      since: %s
`[1:], repeat(since, 15)...))
}

func (s *ListSuite) TestListInitErrors(c *tc.C) {
	s.testListInitError(c, []string{"--filesystem", "--volume"}, "--filesystem and --volume can not be used together")
	s.testListInitError(c, []string{"storage-id"}, "specifying IDs only supported with --filesystem and --volume options")
}

func (s *ListSuite) testListInitError(c *tc.C, args []string, expectedErr string) {
	_, err := s.runList(c, args)
	c.Assert(err, tc.ErrorMatches, expectedErr)
}

func (s *ListSuite) TestListError(c *tc.C) {
	s.mockAPI.listErrors = true
	context, err := s.runList(c, nil)
	c.Assert(err, tc.ErrorMatches, "list fails")
	stderr := cmdtesting.Stderr(context)
	c.Assert(stderr, tc.Equals, "")
	stdout := cmdtesting.Stdout(context)
	c.Assert(stdout, tc.Equals, "")
}

func (s *ListSuite) assertValidList(c *tc.C, args []string, expectedValid string) {
	context, err := s.runList(c, args)
	c.Assert(err, tc.ErrorIsNil)

	obtainedErr := cmdtesting.Stderr(context)
	c.Assert(obtainedErr, tc.Equals, "")

	obtainedValid := cmdtesting.Stdout(context)
	c.Assert(obtainedValid, tc.Equals, expectedValid)
}

type mockListAPI struct {
	listErrors      bool
	listFilesystems func([]string) ([]params.FilesystemDetailsListResult, error)
	listVolumes     func([]string) ([]params.VolumeDetailsListResult, error)
	omitPool        bool
	time            time.Time
}

func (s *mockListAPI) Close() error {
	return nil
}

func (s *mockListAPI) ListStorageDetails(ctx context.Context) ([]params.StorageDetails, error) {
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
			Since:  &s.time,
			Info:   "creating volume",
		},
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-transcode-0": {
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
			Since:  &s.time,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-postgresql-0": {
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
			Since:  &s.time,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-transcode-0": {
				Location: "there",
			},
			"unit-transcode-1": {
				Location: "here",
			},
		},
	}, {
		StorageTag: "storage-persistent-1",
		Kind:       params.StorageKindFilesystem,
		Status: params.EntityStatus{
			Status: status.Detached,
			Since:  &s.time,
		},
		Persistent: true,
	}}
	return results, nil
}

// repeat is used for duplicating the string multiple times.
func repeat(s string, amount int) []interface{} {
	var a []interface{}
	for i := 0; i < amount; i++ {
		a = append(a, s)
	}
	return a
}
