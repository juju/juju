// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/core/description"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/dummy"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type ImportSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&ImportSuite{})

func (s *ImportSuite) SetUpTest(c *gc.C) {
	// Specify the config to use for the controller model before calling
	// SetUpTest of the StateSuite, otherwise we get testing.ModelConfig(c).
	// The default provider type specified in the testing.ModelConfig function
	// is one that isn't registered as a valid provider. For our tests here we
	// need a real registered provider, so we use the dummy provider.
	// NOTE: make a better test provider.
	s.InitialConfig = testing.CustomModelConfig(c, dummy.SampleConfig())
	s.StateSuite.SetUpTest(c)
}

func (s *ImportSuite) TestBadBytes(c *gc.C) {
	bytes := []byte("not a model")
	model, st, err := migration.ImportModel(s.State, bytes)
	c.Check(st, gc.IsNil)
	c.Check(model, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

func (s *ImportSuite) TestImportModel(c *gc.C) {
	model, err := s.State.Export()
	c.Check(err, jc.ErrorIsNil)

	// Update the config values in the exported model for different values for
	// "state-port", "api-port", and "ca-cert". Also give the model a new UUID
	// and name so we can import it nicely.
	uuid := utils.MustNewUUID().String()
	model.UpdateConfig(map[string]interface{}{
		"name": "new-model",
		"uuid": uuid,
	})

	bytes, err := description.Serialize(model)
	c.Check(err, jc.ErrorIsNil)

	dbModel, dbState, err := migration.ImportModel(s.State, bytes)
	c.Check(err, jc.ErrorIsNil)
	defer dbState.Close()

	dbConfig, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dbConfig.UUID(), gc.Equals, uuid)
	c.Assert(dbConfig.Name(), gc.Equals, "new-model")
}

func (s *ImportSuite) TestUploadBinariesConfigValidate(c *gc.C) {
	type T migration.UploadBinariesConfig // alias for brevity

	check := func(modify func(*T), missing string) {
		config := T{
			CharmDownloader: struct{ migration.CharmDownloader }{},
			CharmUploader:   struct{ migration.CharmUploader }{},
			ToolsDownloader: struct{ migration.ToolsDownloader }{},
			ToolsUploader:   struct{ migration.ToolsUploader }{},
		}
		modify(&config)
		realConfig := migration.UploadBinariesConfig(config)
		c.Check(realConfig.Validate(), gc.ErrorMatches, fmt.Sprintf("missing %s not valid", missing))
	}

	check(func(c *T) { c.CharmDownloader = nil }, "CharmDownloader")
	check(func(c *T) { c.CharmUploader = nil }, "CharmUploader")
	check(func(c *T) { c.ToolsDownloader = nil }, "ToolsDownloader")
	check(func(c *T) { c.ToolsUploader = nil }, "ToolsUploader")
}

func (s *ImportSuite) TestBinariesMigration(c *gc.C) {
	downloader := &fakeDownloader{}
	uploader := &fakeUploader{
		charms: make(map[string]string),
		tools:  make(map[version.Binary]string),
	}

	toolsMap := map[version.Binary]string{
		version.MustParseBinary("2.1.0-trusty-amd64"): "/tools/0",
		version.MustParseBinary("2.0.0-xenial-amd64"): "/tools/1",
	}
	config := migration.UploadBinariesConfig{
		Charms:          []string{"local:trusty/magic", "cs:trusty/postgresql-42"},
		CharmDownloader: downloader,
		CharmUploader:   uploader,
		Tools:           toolsMap,
		ToolsDownloader: downloader,
		ToolsUploader:   uploader,
	}
	err := migration.UploadBinaries(config)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(downloader.charms, jc.DeepEquals, []string{
		"local:trusty/magic",
		"cs:trusty/postgresql-42",
	})
	c.Assert(uploader.charms, jc.DeepEquals, map[string]string{
		"local:trusty/magic":      "local:trusty/magic content",
		"cs:trusty/postgresql-42": "cs:trusty/postgresql-42 content",
	})
	c.Assert(downloader.uris, jc.SameContents, []string{
		"/tools/0",
		"/tools/1",
	})
	c.Assert(uploader.tools, jc.DeepEquals, toolsMap)
}

type fakeDownloader struct {
	charms []string
	uris   []string
}

func (d *fakeDownloader) OpenCharm(curl *charm.URL) (io.ReadCloser, error) {
	urlStr := curl.String()
	d.charms = append(d.charms, urlStr)
	// Return the charm URL string as the fake charm content
	return ioutil.NopCloser(bytes.NewReader([]byte(urlStr + " content"))), nil
}

func (d *fakeDownloader) OpenURI(uri string, query url.Values) (io.ReadCloser, error) {
	if query != nil {
		panic("query should be empty")
	}
	d.uris = append(d.uris, uri)
	// Return the URI string as fake content
	return ioutil.NopCloser(bytes.NewReader([]byte(uri))), nil
}

type fakeUploader struct {
	tools  map[version.Binary]string
	charms map[string]string
}

func (f *fakeUploader) UploadTools(r io.ReadSeeker, v version.Binary, _ ...string) (tools.List, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	f.tools[v] = string(data)
	return tools.List{&tools.Tools{Version: v}}, nil
}

func (f *fakeUploader) UploadCharm(u *charm.URL, r io.ReadSeeker) (*charm.URL, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}

	f.charms[u.String()] = string(data)
	return u, nil
}

type ExportSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&ExportSuite{})

func (s *ExportSuite) TestExportModel(c *gc.C) {
	bytes, err := migration.ExportModel(s.State)
	c.Assert(err, jc.ErrorIsNil)
	// The bytes must be a valid model.
	_, err = description.Deserialize(bytes)
	c.Assert(err, jc.ErrorIsNil)
}
