// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
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
	s.PatchValue(storage.GetStorageListAPI, func(c *storage.ListCommand) (storage.StorageListAPI, error) {
		return s.mockAPI, nil
	})

}

func runList(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.ListCommand{}), args...)
}

func (s *ListSuite) TestList(c *gc.C) {
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
[Storage]    
UNIT         ID          LOCATION STATUS  PERSISTENT 
postgresql/0 db-dir/1100          pending false      
transcode/0  db-dir/1000          pending true       
transcode/0  db-dir/1100          pending false      
transcode/0  shared-fs/0          pending false      
transcode/1  shared-fs/0          pending false      

`[1:],
		"",
	)
}

func (s *ListSuite) TestListYAML(c *gc.C) {
	s.assertValidList(
		c,
		[]string{"--format", "yaml"},
		`
postgresql/0:
  db-dir/1100:
    storage: db-dir
    kind: filesystem
    status: pending
    persistent: false
transcode/0:
  db-dir/1000:
    storage: db-dir
    kind: block
    status: pending
    persistent: true
  db-dir/1100:
    storage: db-dir
    kind: filesystem
    status: pending
    persistent: false
  shared-fs/0:
    storage: shared-fs
    kind: unknown
    status: pending
    persistent: false
transcode/1:
  shared-fs/0:
    storage: shared-fs
    kind: unknown
    status: pending
    persistent: false
`[1:],
		"",
	)
}

func (s *ListSuite) TestListOwnerStorageIdSort(c *gc.C) {
	s.mockAPI.lexicalChaos = true
	s.assertValidList(
		c,
		nil,
		// Default format is tabular
		`
[Storage]    
UNIT         ID          LOCATION STATUS  PERSISTENT 
postgresql/0 db-dir/1100          pending false      
transcode/0  db-dir/1000          pending true       
transcode/0  db-dir/1100          pending false      
transcode/0  shared-fs/0          pending false      
transcode/0  shared-fs/5          pending false      
transcode/1  db-dir/1000          pending true       
transcode/1  shared-fs/0          pending false      

`[1:],
		`
error for storage-db-dir-1010
error for test storage-db-dir-1010
`[1:],
	)
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expectedValid, expectedErr string) {
	context, err := runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtainedErr := testing.Stderr(context)
	c.Assert(obtainedErr, gc.Equals, expectedErr)

	obtainedValid := testing.Stdout(context)
	c.Assert(obtainedValid, gc.Equals, expectedValid)
}

type mockListAPI struct {
	lexicalChaos bool
}

func (s mockListAPI) Close() error {
	return nil
}

func (s mockListAPI) List() ([]params.StorageInfo, error) {
	result := []params.StorageInfo{}
	result = append(result, getTestAttachments(s.lexicalChaos)...)
	result = append(result, getTestInstances(s.lexicalChaos)...)
	return result, nil
}

func getTestAttachments(chaos bool) []params.StorageInfo {
	results := []params.StorageInfo{{
		params.StorageDetails{
			StorageTag: "storage-shared-fs-0",
			OwnerTag:   "service-transcode",
			UnitTag:    "unit-transcode-0",
			Kind:       params.StorageKindBlock,
			Location:   "here",
			Status:     "attached",
		}, nil}, {
		params.StorageDetails{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "unit-transcode-0",
			UnitTag:    "unit-transcode-0",
			Kind:       params.StorageKindUnknown,
			Location:   "there",
			Status:     "provisioned",
			Persistent: true,
		}, nil}}

	if chaos {
		last := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-shared-fs-5",
				OwnerTag:   "service-transcode",
				UnitTag:    "unit-transcode-0",
				Kind:       params.StorageKindUnknown,
				Location:   "nowhere",
				Status:     "pending",
			}, nil}
		second := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1010",
				OwnerTag:   "unit-transcode-1",
				UnitTag:    "unit-transcode-1",
				Kind:       params.StorageKindBlock,
				Location:   "",
				Status:     "pending",
			}, &params.Error{Message: "error for storage-db-dir-1010"}}
		first := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				OwnerTag:   "unit-transcode-1",
				UnitTag:    "unit-transcode-1",
				Kind:       params.StorageKindFilesystem,
				Status:     "attached",
				Persistent: true,
			}, nil}
		results = append(results, last)
		results = append(results, second)
		results = append(results, first)
	}
	return results
}

func getTestInstances(chaos bool) []params.StorageInfo {

	results := []params.StorageInfo{
		{
			params.StorageDetails{
				StorageTag: "storage-shared-fs-0",
				OwnerTag:   "service-transcode",
				UnitTag:    "unit-transcode-0",
				Kind:       params.StorageKindUnknown,
				Status:     "pending",
			}, nil},
		{
			params.StorageDetails{
				StorageTag: "storage-shared-fs-0",
				OwnerTag:   "service-transcode",
				UnitTag:    "unit-transcode-1",
				Kind:       params.StorageKindUnknown,
				Status:     "pending",
			}, nil},
		{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1100",
				UnitTag:    "unit-postgresql-0",
				Kind:       params.StorageKindFilesystem,
				Status:     "pending",
			}, nil},
		{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1100",
				UnitTag:    "unit-transcode-0",
				Kind:       params.StorageKindFilesystem,
				Status:     "pending",
			}, nil},
		{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				OwnerTag:   "unit-transcode-0",
				UnitTag:    "unit-transcode-0",
				Kind:       params.StorageKindBlock,
				Status:     "pending",
				Persistent: true,
			}, nil}}

	if chaos {
		last := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-shared-fs-5",
				OwnerTag:   "service-transcode",
				UnitTag:    "unit-transcode-0",
				Kind:       params.StorageKindUnknown,
				Status:     "pending",
			}, nil}
		second := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1010",
				UnitTag:    "unit-transcode-1",
				Kind:       params.StorageKindBlock,
				Status:     "pending",
			}, &params.Error{Message: "error for test storage-db-dir-1010"}}
		first := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				UnitTag:    "unit-transcode-1",
				Kind:       params.StorageKindFilesystem,
				Status:     "pending",
				Persistent: true,
			}, nil}
		zero := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1100",
				UnitTag:    "unit-postgresql-0",
				Kind:       params.StorageKindFilesystem,
				Status:     "pending",
			}, nil}
		results = append(results, last)
		results = append(results, second)
		results = append(results, zero)
		results = append(results, first)
	}
	return results
}
