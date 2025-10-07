// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/api/client/application"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/storage"
	jujutesting "github.com/juju/juju/testing"
)

type StorageConfigSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite
	store   *jujuclient.MemStore
	mockAPI *mockStorageConstraintsAPI
}

var _ = gc.Suite(&StorageConfigSuite{})

func (s *StorageConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.CurrentControllerName = "testing"
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{}
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin/controller": {},
		},
		CurrentModel: "admin/controller",
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User: "admin",
	}

	// Default mock returns one application with two storage keys.
	s.mockAPI = &mockStorageConstraintsAPI{
		getFunc: func(app string) (apiapplication.ApplicationStorageDirectives, error) {
			return apiapplication.ApplicationStorageDirectives{
				StorageDirectives: map[string]storage.Constraints{
					"data":    {Pool: "rootfs", Size: 10240, Count: 1}, // 10 GiB (MiB units)
					"allecto": {Pool: "loop", Size: 20480, Count: 2},   // 20 GiB
				},
			}, nil
		},
		updateFunc: func(up apiapplication.ApplicationStorageUpdate) error {
			return nil
		},
	}
}

func (s *StorageConfigSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, application.NewStorageCommandForTest(s.mockAPI, s.store), args...)
}

func (s *StorageConfigSuite) TestNoArguments(c *gc.C) {
	ctx, err := s.run(c)
	c.Assert(err, gc.ErrorMatches, "no application specified")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "ERROR no application specified\n")
}

func (s *StorageConfigSuite) TestGetConstraintsInvalidApplicationName(c *gc.C) {
	ctx, err := s.run(c, "so-42-far-not-good")
	c.Assert(err, gc.ErrorMatches, `invalid application name "so-42-far-not-good"`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "ERROR invalid application name \"so-42-far-not-good\"\n")
}

func (s *StorageConfigSuite) TestGetConstraintsAllJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "storage-block")
	c.Assert(err, jc.ErrorIsNil)

	want := "" +
		"{\"allecto\":{\"Pool\":\"loop\",\"Size\":20480,\"Count\":2}," +
		"\"data\":{\"Pool\":\"rootfs\",\"Size\":10240,\"Count\":1}}\n"

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *StorageConfigSuite) TestGetConstraintsAllYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml", "storage-block")
	c.Assert(err, jc.ErrorIsNil)

	want := "" +
		"allecto:\n" +
		"  pool: loop\n" +
		"  size: 20480\n" +
		"  count: 2\n" +
		"data:\n" +
		"  pool: rootfs\n" +
		"  size: 10240\n" +
		"  count: 1\n"

	output := cmdtesting.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *StorageConfigSuite) TestGetConstraintsAllTabular(c *gc.C) {
	ctx, err := s.run(c, "storage-block")
	c.Assert(err, jc.ErrorIsNil)

	out := strings.TrimSpace(cmdtesting.Stdout(ctx))
	lines := strings.Split(out, "\n")
	c.Assert(len(lines), gc.Equals, 3)

	assertTabLine(c, lines[0], "Storage", "Pool", "Size", "Count")
	assertTabLine(c, lines[1], "allecto", "loop", "20480", "2")
	assertTabLine(c, lines[2], "data", "rootfs", "10240", "1")

	c.Assert(strings.TrimSpace(cmdtesting.Stderr(ctx)), gc.Equals, "")
}

func (s *StorageConfigSuite) TestGetConstraintsSingleKeyTabular(c *gc.C) {
	ctx, err := s.run(c, "storage-block", "data")
	c.Assert(err, jc.ErrorIsNil)

	out := strings.TrimSpace(cmdtesting.Stdout(ctx))
	lines := strings.Split(out, "\n")
	c.Assert(len(lines), gc.Equals, 2)

	assertTabLine(c, lines[0], "Storage", "Pool", "Size", "Count")
	assertTabLine(c, lines[1], "data", "rootfs", "10240", "1")

	c.Assert(strings.TrimSpace(cmdtesting.Stderr(ctx)), gc.Equals, "")
}

func (s *StorageConfigSuite) TestGetConstraintsSingleKeyJSON(c *gc.C) {
	ctx, err := s.run(c, "storage-block", "data", "--format", "json")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"data":{"Pool":"rootfs","Size":10240,"Count":1}}`+"\n")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *StorageConfigSuite) TestGetConstraintsSingleKeyYAML(c *gc.C) {
	ctx, err := s.run(c, "storage-block", "data", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, ""+
		"data:\n"+
		"  pool: rootfs\n"+
		"  size: 10240\n"+
		"  count: 1\n")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *StorageConfigSuite) TestGetConstraintsKeyNotFound(c *gc.C) {
	ctx, err := s.run(c, "storage-block", "doesnotexist")
	c.Assert(err, gc.ErrorMatches, `storage "doesnotexist" not found`)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *StorageConfigSuite) TestGetConstraintsAPIError(c *gc.C) {
	s.mockAPI.getFunc = func(app string) (apiapplication.ApplicationStorageDirectives, error) {
		return apiapplication.ApplicationStorageDirectives{}, fmt.Errorf("api error")
	}
	_, err := s.run(c, "storage-block")
	c.Assert(err, gc.ErrorMatches, "api error")
}

func (s *StorageConfigSuite) TestSetConstraintsAllFields(c *gc.C) {
	var got apiapplication.ApplicationStorageUpdate
	s.mockAPI.updateFunc = func(up apiapplication.ApplicationStorageUpdate) error {
		got = up
		return nil
	}

	ctx, err := s.run(c, "storage-block", "data=100G,rootfs,1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(cmdtesting.Stderr(ctx)), gc.Equals, "")

	// Assert application tag.
	c.Assert(got.ApplicationTag, gc.Equals, names.NewApplicationTag("storage-block"))

	data := got.StorageDirectives["data"]
	c.Assert(data.Pool, gc.Equals, "rootfs")
	c.Assert(data.Count, gc.Equals, uint64(1))
	c.Assert(data.Size, gc.Equals, uint64(102400))
}

func (s *StorageConfigSuite) TestSetConstraintsAllFieldsShuffledOrder(c *gc.C) {
	var got apiapplication.ApplicationStorageUpdate
	s.mockAPI.updateFunc = func(up apiapplication.ApplicationStorageUpdate) error {
		got = up
		return nil
	}

	ctx, err := s.run(c, "storage-block", "data=rootfs,1,100G")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(cmdtesting.Stderr(ctx)), gc.Equals, "")

	// Assert application tag.
	c.Assert(got.ApplicationTag, gc.Equals, names.NewApplicationTag("storage-block"))

	data := got.StorageDirectives["data"]
	c.Assert(data.Pool, gc.Equals, "rootfs")
	c.Assert(data.Count, gc.Equals, uint64(1))
	c.Assert(data.Size, gc.Equals, uint64(102400))
}

func (s *StorageConfigSuite) TestSetConstraintsOneField(c *gc.C) {
	var got apiapplication.ApplicationStorageUpdate
	s.mockAPI.updateFunc = func(up apiapplication.ApplicationStorageUpdate) error {
		got = up
		return nil
	}

	// Only pool provided.
	ctx, err := s.run(c, "storage-block", "data=,rootfs,")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(cmdtesting.Stderr(ctx)), gc.Equals, "")

	c.Assert(got.ApplicationTag, gc.Equals, names.NewApplicationTag("storage-block"))

	data := got.StorageDirectives["data"]
	c.Assert(data.Pool, gc.Equals, "rootfs")
	c.Assert(data.Count, gc.Equals, uint64(0))
	c.Assert(data.Size, gc.Equals, uint64(0))
}

func (s *StorageConfigSuite) TestSetConstraintsMultipleStorageKeys(c *gc.C) {
	var got apiapplication.ApplicationStorageUpdate
	s.mockAPI.updateFunc = func(up apiapplication.ApplicationStorageUpdate) error {
		got = up
		return nil
	}

	ctx, err := s.run(c, "storage-block", "data=100G,rootfs,1", "allecto=,loop,2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(strings.TrimSpace(cmdtesting.Stderr(ctx)), gc.Equals, "")

	c.Assert(got.ApplicationTag, gc.Equals, names.NewApplicationTag("storage-block"))

	data := got.StorageDirectives["data"]
	c.Assert(data.Pool, gc.Equals, "rootfs")
	c.Assert(data.Count, gc.Equals, uint64(1))
	c.Assert(data.Size, gc.Equals, uint64(102400))

	allecto := got.StorageDirectives["allecto"]
	c.Assert(allecto.Pool, gc.Equals, "loop")
	c.Assert(allecto.Count, gc.Equals, uint64(2))
	c.Assert(allecto.Size, gc.Equals, uint64(0))
}

func (s *StorageConfigSuite) TestSetConstraintsAPIError(c *gc.C) {
	s.mockAPI.updateFunc = func(up apiapplication.ApplicationStorageUpdate) error {
		return fmt.Errorf("api error")
	}
	_, err := s.run(c, "storage-block", "data=100G,rootfs,1")
	c.Assert(err, gc.ErrorMatches, "api error")
}

func (s *StorageConfigSuite) TestSetConstraintsParseError(c *gc.C) {
	_, err := s.run(c, "storage-block", "data=not-a-size,rootfs,one")
	c.Assert(err, gc.ErrorMatches, `parsing storage constraints for "data": .*`)
}

func assertTabLine(c *gc.C, line string, fields ...string) {
	cols := strings.Fields(strings.TrimSpace(line))
	c.Assert(cols, gc.DeepEquals, fields)
}

// Mocks
type mockStorageConstraintsAPI struct {
	getFunc    func(app string) (apiapplication.ApplicationStorageDirectives, error)
	updateFunc func(apiapplication.ApplicationStorageUpdate) error
}

func (m *mockStorageConstraintsAPI) Close() error { return nil }

func (m *mockStorageConstraintsAPI) GetApplicationStorageDirectives(applicationName string) (apiapplication.ApplicationStorageDirectives, error) {
	return m.getFunc(applicationName)
}

func (m *mockStorageConstraintsAPI) UpdateApplicationStorageDirectives(up apiapplication.ApplicationStorageUpdate) error {
	return m.updateFunc(up)
}
