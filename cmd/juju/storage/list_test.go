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
	return testing.RunCommand(c, storage.NewListCommand(s.mockAPI, s.store), args...)
}

func (s *ListSuite) TestList(c *gc.C) {
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
\[Storage\]    
UNIT         ID          LOCATION STATUS   MESSAGE 
postgresql/0 db-dir/1100 hither   attached         
transcode/0  db-dir/1000 thither  pending          
transcode/0  shared-fs/0 there    attached         
transcode/1  shared-fs/0 here     attached         

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
      since: .*
    persistent: false
    attachments:
      units:
        transcode/0:
          location: thither
  db-dir/1100:
    kind: block
    status:
      current: attached
      since: .*
    persistent: true
    attachments:
      units:
        postgresql/0:
          location: hither
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
`[1:])
}

func (s *ListSuite) TestListOwnerStorageIdSort(c *gc.C) {
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
\[Storage\]    
UNIT         ID          LOCATION STATUS   MESSAGE 
postgresql/0 db-dir/1100 hither   attached         
transcode/0  db-dir/1000 thither  pending          
transcode/0  shared-fs/0 there    attached         
transcode/1  shared-fs/0 here     attached         

`[1:])
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
	listErrors bool
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) ListStorageDetails() ([]params.StorageDetails, error) {
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
			Status: params.StatusPending,
			Since:  &epoch,
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
		Status: params.EntityStatus{
			Status: params.StatusAttached,
			Since:  &epoch,
		},
		Persistent: true,
		Attachments: map[string]params.StorageAttachmentDetails{
			"unit-postgresql-0": params.StorageAttachmentDetails{
				Location: "hither",
			},
		},
	}, {
		StorageTag: "storage-shared-fs-0",
		OwnerTag:   "service-transcode",
		Kind:       params.StorageKindFilesystem,
		Status: params.EntityStatus{
			Status: params.StatusAttached,
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
