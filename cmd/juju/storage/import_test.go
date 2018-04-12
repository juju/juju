// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"errors"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/juju/storage"
	_ "github.com/juju/juju/provider/dummy"
	jujustorage "github.com/juju/juju/storage"
)

type ImportFilesystemSuite struct {
	SubStorageSuite
	importer mockStorageImporter
}

var _ = gc.Suite(&ImportFilesystemSuite{})

func (s *ImportFilesystemSuite) SetUpTest(c *gc.C) {
	s.SubStorageSuite.SetUpTest(c)
	s.importer = mockStorageImporter{}
}

var initErrorTests = []struct {
	args        []string
	expectedErr string
}{{
	args:        []string{"foo", "bar"},
	expectedErr: "import-filesystem requires a storage provider, provider ID, and storage name",
}, {
	args:        []string{"123", "foo", "bar"},
	expectedErr: `pool name "123" not valid`,
}, {
	args:        []string{"foo", "abc123", "123"},
	expectedErr: `"123" is not a valid storage name`,
}}

func (s *ImportFilesystemSuite) TestInitErrors(c *gc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d for %q", i, t.args)
		_, err := s.run(c, t.args...)
		c.Assert(err, gc.ErrorMatches, t.expectedErr)
	}
}

func (s *ImportFilesystemSuite) TestImportSuccess(c *gc.C) {
	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
importing "bar" from storage pool "foo" as storage "baz"
imported storage baz/0
`[1:])

	s.importer.CheckCalls(c, []testing.StubCall{
		{"ImportStorage", []interface{}{
			jujustorage.StorageKindFilesystem,
			"foo", "bar", "baz",
		}},
		{"Close", nil},
	})
}

func (s *ImportFilesystemSuite) TestImportError(c *gc.C) {
	s.importer.SetErrors(errors.New("nope"))

	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, gc.ErrorMatches, "nope")

	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `importing "bar" from storage pool "foo" as storage "baz"`+"\n")
}

func (s *ImportFilesystemSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewImportFilesystemCommand(
		func(*storage.StorageCommandBase) (storage.StorageImporter, error) {
			return &s.importer, nil
		},
		s.store,
	), args...)
}

type mockStorageImporter struct {
	testing.Stub
}

func (m *mockStorageImporter) Close() error {
	m.MethodCall(m, "Close")
	return m.NextErr()
}

func (m *mockStorageImporter) ImportStorage(
	k jujustorage.StorageKind,
	pool, providerId, storageName string,
) (names.StorageTag, error) {
	m.MethodCall(m, "ImportStorage", k, pool, providerId, storageName)
	return names.NewStorageTag(storageName + "/0"), m.NextErr()
}
