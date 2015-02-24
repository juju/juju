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
OWNER        ID          NAME      ATTACHED    LOCATION KIND    
postgresql/0 db-dir/1000 db-dir    transcode/0 there    unknown 
transcode    db-dir/1000 db-dir                         block   
transcode    shared-fs/0 shared-fs                      unknown 
transcode/0  shared-fs/0 shared-fs transcode/0 here     block   

`[1:],
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
    unit_id: transcode/0
    attached: true
    location: there
    provisioned: true
transcode:
  db-dir/1000:
    storage: db-dir
    kind: block
  shared-fs/0:
    storage: shared-fs
    kind: unknown
transcode/0:
  shared-fs/0:
    storage: shared-fs
    kind: block
    unit_id: transcode/0
    attached: true
    location: here
    provisioned: true
`[1:],
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
OWNER        ID          NAME      ATTACHED    LOCATION KIND        
postgresql/0 db-dir/1000 db-dir    transcode/0 there    unknown     
transcode    db-dir/1000 db-dir                         file system 
transcode    shared-fs/0 shared-fs                      unknown     
transcode    shared-fs/5 shared-fs                      unknown     
transcode/0  db-dir/1000 db-dir    transcode/0          block       
transcode/0  shared-fs/0 shared-fs transcode/0 here     block       
transcode/0  shared-fs/5 shared-fs transcode/0 nowhere  unknown     

`[1:],
	)
}

func (s *ListSuite) assertValidList(c *gc.C, args []string, expected string) {
	context, err := runList(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
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
		StorageTag:  "storage-shared-fs-0",
		OwnerTag:    "unit-transcode-0",
		UnitTag:     "unit-transcode-0",
		Kind:        params.StorageKindBlock,
		Location:    "here",
		Provisioned: true,
		Attached:    true,
	}, {
		StorageTag:  "storage-db-dir-1000",
		OwnerTag:    "unit-postgresql-0",
		UnitTag:     "unit-transcode-0",
		Kind:        params.StorageKindUnknown,
		Location:    "there",
		Provisioned: true,
		Attached:    true,
	}}

	if chaos {
		last := params.StorageInfo{
			StorageTag:  "storage-shared-fs-5",
			OwnerTag:    "unit-transcode-0",
			UnitTag:     "unit-transcode-0",
			Kind:        params.StorageKindUnknown,
			Location:    "nowhere",
			Provisioned: true,
			Attached:    true,
		}
		second := params.StorageInfo{
			StorageTag:  "storage-db-dir-1000",
			OwnerTag:    "unit-transcode-0",
			UnitTag:     "unit-transcode-0",
			Kind:        params.StorageKindBlock,
			Location:    "",
			Provisioned: true,
			Attached:    true,
		}
		first := params.StorageInfo{
			StorageTag:  "storage-db-dir-1000",
			OwnerTag:    "service-transcode",
			UnitTag:     "unit-transcode-0",
			Kind:        params.StorageKindFilesystem,
			Provisioned: true,
			Attached:    true,
		}
		results = append(results, last)
		results = append(results, second)
		results = append(results, first)
	}
	return results
}

func getTestInstances(chaos bool) []params.StorageInfo {

	results := []params.StorageInfo{{
		StorageTag: "storage-shared-fs-0",
		OwnerTag:   "service-transcode",
		Kind:       params.StorageKindUnknown,
	}, {
		StorageTag: "storage-db-dir-1000",
		OwnerTag:   "service-transcode",
		Kind:       params.StorageKindBlock,
	}}

	if chaos {
		last := params.StorageInfo{
			StorageTag: "storage-shared-fs-5",
			OwnerTag:   "service-transcode",
			Kind:       params.StorageKindUnknown,
		}
		second := params.StorageInfo{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "service-transcode",
			Kind:       params.StorageKindBlock,
		}
		first := params.StorageInfo{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "service-transcode",
			Kind:       params.StorageKindFilesystem,
		}
		results = append(results, last)
		results = append(results, second)
		results = append(results, first)
	}
	return results
}
