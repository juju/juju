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
	mockAPI *mockShowAPI
}

var _ = gc.Suite(&ShowSuite{})

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)

	s.mockAPI = &mockShowAPI{}
	s.PatchValue(storage.GetStorageShowAPI, func(c *storage.ShowCommand) (storage.StorageShowAPI, error) {
		return s.mockAPI, nil
	})

}

func runShow(c *gc.C, args []string) (*cmd.Context, error) {
	return testing.RunCommand(c, envcmd.Wrap(&storage.ShowCommand{}), args...)
}

func (s *ShowSuite) TestShowNoMatch(c *gc.C) {
	s.mockAPI.noMatch = true
	s.assertValidShow(
		c,
		[]string{"fluff/0"},
		`
{}
`[1:],
	)
}

func (s *ShowSuite) TestShow(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0"},
		// Default format is yaml
		`
postgresql:
  shared-fs/0:
    storage: shared-fs
    kind: block
    status: pending
postgresql/0:
  shared-fs/0:
    storage: shared-fs
    kind: block
    unit_id: postgresql/0
    status: attached
    location: a location
`[1:],
	)
}

func (s *ShowSuite) TestShowInvalidId(c *gc.C) {
	_, err := runShow(c, []string{"foo"})
	c.Assert(err, gc.ErrorMatches, ".*invalid storage id foo.*")
}

func (s *ShowSuite) TestShowJSON(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "--format", "json"},
		`{"postgresql":{"shared-fs/0":{"storage":"shared-fs","kind":"block","status":"pending"}},"postgresql/0":{"shared-fs/0":{"storage":"shared-fs","kind":"block","unit_id":"postgresql/0","status":"attached","location":"a location"}}}
`,
	)
}

func (s *ShowSuite) TestShowMultipleReturn(c *gc.C) {
	s.assertValidShow(
		c,
		[]string{"shared-fs/0", "db-dir/1000"},
		`
postgresql:
  db-dir/1000:
    storage: db-dir
    kind: block
    status: pending
  shared-fs/0:
    storage: shared-fs
    kind: block
    status: pending
postgresql/0:
  db-dir/1000:
    storage: db-dir
    kind: block
    unit_id: postgresql/0
    status: attached
    location: a location
  shared-fs/0:
    storage: shared-fs
    kind: block
    unit_id: postgresql/0
    status: attached
    location: a location
`[1:],
	)
}

func (s *ShowSuite) assertValidShow(c *gc.C, args []string, expected string) {
	context, err := runShow(c, args)
	c.Assert(err, jc.ErrorIsNil)

	obtained := testing.Stdout(context)
	c.Assert(obtained, gc.Equals, expected)
}

type mockShowAPI struct {
	noMatch bool
}

func (s mockShowAPI) Close() error {
	return nil
}

func (s mockShowAPI) Show(tags []names.StorageTag) ([]params.StorageDetails, error) {
	if s.noMatch {
		return nil, nil
	}
	all := make([]params.StorageDetails, len(tags)*2)
	ind := 0
	for _, tag := range tags {
		all[ind] = params.StorageDetails{
			StorageTag: tag.String(),
			OwnerTag:   "service-postgresql",
			Kind:       params.StorageKindBlock,
		}
		ind++
	}
	for _, tag := range tags {
		all[ind] = params.StorageDetails{
			StorageTag: tag.String(),
			OwnerTag:   "unit-postgresql-0",
			UnitTag:    "unit-postgresql-0",
			Kind:       params.StorageKindBlock,
			Location:   "a location",
			Status:     params.StorageStatusAttached,
		}
		ind++
	}
	return all, nil
}
