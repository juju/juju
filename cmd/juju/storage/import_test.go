// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	jujustorage "github.com/juju/juju/internal/storage"
)

type ImportFilesystemSuite struct {
	SubStorageSuite
	importer mockStorageImporter
}

var _ = tc.Suite(&ImportFilesystemSuite{})

func (s *ImportFilesystemSuite) SetUpTest(c *tc.C) {
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

func (s *ImportFilesystemSuite) TestInitErrors(c *tc.C) {
	for i, t := range initErrorTests {
		c.Logf("test %d for %q", i, t.args)
		_, err := s.run(c, t.args...)
		c.Assert(err, tc.ErrorMatches, t.expectedErr)
	}
}

func (s *ImportFilesystemSuite) TestImportSuccess(c *tc.C) {
	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `
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

func (s *ImportFilesystemSuite) TestImportError(c *tc.C) {
	s.importer.SetErrors(errors.New("nope"))

	ctx, err := s.run(c, "foo", "bar", "baz")
	c.Assert(err, tc.ErrorMatches, "nope")

	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, `importing "bar" from storage pool "foo" as storage "baz"`+"\n")
}

func (s *ImportFilesystemSuite) run(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, storage.NewImportFilesystemCommand(
		func(context.Context, *storage.StorageCommandBase) (storage.StorageImporter, error) {
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
	ctx context.Context,
	k jujustorage.StorageKind,
	pool, providerId, storageName string,
) (names.StorageTag, error) {
	m.MethodCall(m, "ImportStorage", k, pool, providerId, storageName)
	return names.NewStorageTag(storageName + "/0"), m.NextErr()
}
