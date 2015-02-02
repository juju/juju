// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/cmd"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type ShowSuite struct {
	SubStorageSuite
	mockAPI *mockStorageAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockStorageAPI{}
	s.PatchValue(storage.GetStorageShowAPI, func(c *storage.ShowCommand) (storage.StorageShowAPI, error) {
		return s.mockAPI, nil
	})

}

func runShow(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.ShowCommand{}), args...)
}

func (s *ShowSuite) TestShow(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0"},
		// Default format is yaml
		`- storage-tag: storage-shared-fs-0
  storage-name: storage-name
  owner-tag: unitTag
  location: witty
  available-size: 30
  total-size: 100
  tags:
  - tests
  - well
  - maybe
`,
	)
}

func (s *ShowSuite) TestShowJSON(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "--format", "json"},
		`[{"storage-tag":"storage-shared-fs-0","storage-name":"storage-name","owner-tag":"unitTag","location":"witty","available-size":30,"total-size":100,"tags":["tests","well","maybe"]}]
`,
	)
}

func (s *ShowSuite) TestShowMultipleReturn(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "db-dir/1000"},
		`- storage-tag: storage-shared-fs-0
  storage-name: storage-name
  owner-tag: unitTag
  location: witty
  available-size: 30
  total-size: 100
  tags:
  - tests
  - well
  - maybe
- storage-tag: storage-db-dir-1000
  storage-name: storage-name
  owner-tag: unitTag
  location: witty
  available-size: 30
  total-size: 100
  tags:
  - tests
  - well
  - maybe
`,
	)
}

func (s *ShowSuite) assertValidShow(c *gc.C, args []string, expected string) {
	context, err := runShow(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockStorageAPI struct {
}

func (s mockStorageAPI) Close() error {
	return nil
}

func (s mockStorageAPI) Show(tags []names.StorageTag) ([]params.StorageInstance, error) {
	results := make([]params.StorageInstance, len(tags))

	for i, tag := range tags {
		results[i] = params.StorageInstance{
			StorageTag:    tag.String(),
			StorageName:   "storage-name",
			OwnerTag:      "unitTag",
			Location:      "witty",
			AvailableSize: 30,
			TotalSize:     100,
			Tags:          []string{"tests", "well", "maybe"},
		}
	}
	return results, nil
}
