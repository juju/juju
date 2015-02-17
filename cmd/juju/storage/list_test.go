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
OWNER        ID          
postgresql/0 db-dir/1000 
transcode/0  shared-fs/0 

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
transcode/0:
  shared-fs/0:
    storage: shared-fs
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
OWNER        ID          
postgresql/0 db-dir/1000 
transcode    db-dir/1000 
transcode/0  db-dir/1000 
transcode/0  shared-fs/0 
transcode/0  shared-fs/5 

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

func (s mockListAPI) List() ([]params.StorageInstance, error) {

	results := []params.StorageInstance{{
		StorageTag: "storage-shared-fs-0",
		OwnerTag:   "unit-transcode-0",
	}, {
		StorageTag: "storage-db-dir-1000",
		OwnerTag:   "unit-postgresql-0",
	}}

	if s.lexicalChaos {
		last := params.StorageInstance{
			StorageTag: "storage-shared-fs-5",
			OwnerTag:   "unit-transcode-0",
		}
		second := params.StorageInstance{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "unit-transcode-0",
		}
		first := params.StorageInstance{
			StorageTag: "storage-db-dir-1000",
			OwnerTag:   "service-transcode",
		}
		results = append(results, last)
		results = append(results, second)
		results = append(results, first)
	}
	return results, nil
}
