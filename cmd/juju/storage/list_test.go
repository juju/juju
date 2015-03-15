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
UNIT        ID          LOCATION 
            db-dir/1000 there    
            db-dir/1000          
            shared-fs/0          
transcode/0 shared-fs/0 here     

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
  db-dir/1000:
    storage: db-dir
    kind: unknown
    attached_status: unknown
    location: there
    provisioned_status: provisioned
transcode:
  db-dir/1000:
    storage: db-dir
    kind: block
    attached_status: unknown
    provisioned_status: unknown
  shared-fs/0:
    storage: shared-fs
    kind: unknown
    attached_status: unknown
    provisioned_status: unknown
transcode/0:
  shared-fs/0:
    storage: shared-fs
    kind: block
    unit_id: transcode/0
    attached_status: attached
    location: here
    provisioned_status: pending
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
UNIT        ID          LOCATION 
            db-dir/1000 there    
            db-dir/1000          
            shared-fs/0          
            shared-fs/5          
transcode/0 shared-fs/0 here     
transcode/0 shared-fs/5 nowhere  

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
			StorageTag:  "storage-shared-fs-0",
			OwnerTag:    "unit-transcode-0",
			UnitTag:     "unit-transcode-0",
			Kind:        params.StorageKindBlock,
			Location:    "here",
			Provisioned: params.StorageProvisionedStatusPending,
			Attached:    params.StorageAttachedStatusAttached,
		}, nil}, {
		params.StorageDetails{
			StorageTag:  "storage-db-dir-1000",
			OwnerTag:    "unit-postgresql-0",
			UnitTag:     "unit-transcode-0",
			Kind:        params.StorageKindUnknown,
			Location:    "there",
			Provisioned: params.StorageProvisionedStatusProvisioned,
			Attached:    params.StorageAttachedStatusUnknown,
		}, nil}}

	if chaos {
		last := params.StorageInfo{
			params.StorageDetails{
				StorageTag:  "storage-shared-fs-5",
				OwnerTag:    "unit-transcode-0",
				UnitTag:     "unit-transcode-0",
				Kind:        params.StorageKindUnknown,
				Location:    "nowhere",
				Provisioned: params.StorageProvisionedStatusProvisioned,
				Attached:    params.StorageAttachedStatusAttached,
			}, nil}
		second := params.StorageInfo{
			params.StorageDetails{
				StorageTag:  "storage-db-dir-1010",
				OwnerTag:    "unit-transcode-0",
				UnitTag:     "unit-transcode-0",
				Kind:        params.StorageKindBlock,
				Location:    "",
				Provisioned: params.StorageProvisionedStatusUnknown,
				Attached:    params.StorageAttachedStatusUnknown,
			}, &params.Error{Message: "error for storage-db-dir-1010"}}
		first := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				OwnerTag:   "service-transcode",
				UnitTag:    "unit-transcode-0",
				Kind:       params.StorageKindFilesystem,
				Attached:   params.StorageAttachedStatusAttached,
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
				Kind:       params.StorageKindUnknown,
			}, nil},
		{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				OwnerTag:   "service-transcode",
				Kind:       params.StorageKindBlock,
			}, nil}}

	if chaos {
		last := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-shared-fs-5",
				OwnerTag:   "service-transcode",
				Kind:       params.StorageKindUnknown,
			}, nil}
		second := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1010",
				OwnerTag:   "service-transcode",
				Kind:       params.StorageKindBlock,
			}, &params.Error{Message: "error for test storage-db-dir-1010"}}
		first := params.StorageInfo{
			params.StorageDetails{
				StorageTag: "storage-db-dir-1000",
				OwnerTag:   "service-transcode",
				Kind:       params.StorageKindFilesystem,
			}, nil}
		results = append(results, last)
		results = append(results, second)
		results = append(results, first)
	}
	return results
}
