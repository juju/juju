// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/leadership"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/resources"
	resourcetesting "github.com/juju/juju/core/resources/testing"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/tools"
)

type ImportSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&ImportSuite{})

func (s *ImportSuite) SetUpTest(c *gc.C) {
	// Specify the config to use for the controller model before
	// calling SetUpTest of the StateSuite, otherwise we get
	// coretesting.ModelConfig(c). The default provider type
	// specified in the coretesting.ModelConfig function is one that
	// isn't registered as a valid provider. For our tests here we
	// need a real registered provider, so we use the dummy provider.
	// NOTE: make a better test provider.
	s.InitialConfig = coretesting.CustomModelConfig(c, dummy.SampleConfig())
	s.StateSuite.SetUpTest(c)
}

func (s *ImportSuite) TestBadBytes(c *gc.C) {
	bytes := []byte("not a model")
	controller := state.NewController(s.StatePool)
	model, st, err := migration.ImportModel(controller, fakeGetClaimer, bytes)
	c.Check(st, gc.IsNil)
	c.Check(model, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "yaml: unmarshal errors:\n.*")
}

func (s *ImportSuite) exportImport(c *gc.C, leaders map[string]string, getClaimer migration.ClaimerFunc) *state.State {
	model, err := s.State.Export(leaders)
	c.Assert(err, jc.ErrorIsNil)

	// Update the config values in the exported model for different values for
	// "state-port", "api-port", and "ca-cert". Also give the model a new UUID
	// and name, so we can import it nicely.
	uuid := utils.MustNewUUID().String()
	model.UpdateConfig(map[string]interface{}{
		"name": "new-model",
		"uuid": uuid,
	})

	bytes, err := description.Serialize(model)
	c.Check(err, jc.ErrorIsNil)

	controller := state.NewController(s.StatePool)
	dbModel, dbState, err := migration.ImportModel(controller, getClaimer, bytes)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { dbState.Close() })

	dbConfig, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dbConfig.UUID(), gc.Equals, uuid)
	c.Assert(dbConfig.Name(), gc.Equals, "new-model")
	return dbState
}

func (s *ImportSuite) TestImportModel(c *gc.C) {
	s.exportImport(c, map[string]string{}, fakeGetClaimer)
}

func (s *ImportSuite) TestImportsLeadership(c *gc.C) {
	s.makeApplicationWithUnits(c, "wordpress", 3)
	s.makeApplicationWithUnits(c, "mysql", 2)
	leaders := map[string]string{"wordpress": "wordpress/1"}

	var (
		claimer   fakeClaimer
		modelUUID string
	)
	dbState := s.exportImport(c, leaders, func(uuid string) (leadership.Claimer, error) {
		modelUUID = uuid
		return &claimer, nil
	})
	c.Assert(modelUUID, gc.Equals, dbState.ModelUUID())
	c.Assert(claimer.stub.Calls(), gc.HasLen, 1)
	claimer.stub.CheckCall(c, 0, "ClaimLeadership", "wordpress", "wordpress/1", time.Minute)
}

func (s *ImportSuite) makeApplicationWithUnits(c *gc.C, applicationname string, count int) {
	units := make([]*state.Unit, count)
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: applicationname,
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: applicationname,
		}),
	})
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{
			Application: application,
		})
	}
}

func (s *ImportSuite) TestUploadBinariesConfigValidate(c *gc.C) {
	type T migration.UploadBinariesConfig // alias for brevity

	check := func(modify func(*T), missing string) {
		config := T{
			CharmDownloader:    struct{ migration.CharmDownloader }{},
			CharmUploader:      struct{ migration.CharmUploader }{},
			ToolsDownloader:    struct{ migration.ToolsDownloader }{},
			ToolsUploader:      struct{ migration.ToolsUploader }{},
			ResourceDownloader: struct{ migration.ResourceDownloader }{},
			ResourceUploader:   struct{ migration.ResourceUploader }{},
		}
		modify(&config)
		realConfig := migration.UploadBinariesConfig(config)
		c.Check(realConfig.Validate(), gc.ErrorMatches, fmt.Sprintf("missing %s not valid", missing))
	}

	check(func(c *T) { c.CharmDownloader = nil }, "CharmDownloader")
	check(func(c *T) { c.CharmUploader = nil }, "CharmUploader")
	check(func(c *T) { c.ToolsDownloader = nil }, "ToolsDownloader")
	check(func(c *T) { c.ToolsUploader = nil }, "ToolsUploader")
	check(func(c *T) { c.ResourceDownloader = nil }, "ResourceDownloader")
	check(func(c *T) { c.ResourceUploader = nil }, "ResourceUploader")
}

func (s *ImportSuite) TestBinariesMigration(c *gc.C) {
	downloader := &fakeDownloader{}
	uploader := &fakeUploader{
		tools:     make(map[version.Binary]string),
		resources: make(map[string]string),
	}

	toolsMap := map[version.Binary]string{
		version.MustParseBinary("2.1.0-ubuntu-amd64"): "/tools/0",
		version.MustParseBinary("2.0.0-ubuntu-amd64"): "/tools/1",
	}

	app0Res := resourcetesting.NewResource(c, nil, "blob0", "app0", "blob0").Resource
	app1Res := resourcetesting.NewResource(c, nil, "blob1", "app1", "blob1").Resource
	app1UnitRes := app1Res
	app1UnitRes.Revision = 1
	app2Res := resourcetesting.NewPlaceholderResource(c, "blob2", "app2")
	resources := []coremigration.SerializedModelResource{
		{ApplicationRevision: app0Res},
		{
			ApplicationRevision: app1Res,
			UnitRevisions:       map[string]resources.Resource{"app1/99": app1UnitRes},
		},
		{ApplicationRevision: app2Res},
	}

	config := migration.UploadBinariesConfig{
		Charms: []string{
			// These 2 are out of order. Rev 2 must be uploaded first.
			"local:trusty/magic-10",
			"local:trusty/magic-2",
			"ch:trusty/postgresql-42",
		},
		CharmDownloader:    downloader,
		CharmUploader:      uploader,
		Tools:              toolsMap,
		ToolsDownloader:    downloader,
		ToolsUploader:      uploader,
		Resources:          resources,
		ResourceDownloader: downloader,
		ResourceUploader:   uploader,
	}
	err := migration.UploadBinaries(config)
	c.Assert(err, jc.ErrorIsNil)

	expectedCurls := []string{
		// Note ordering.
		"ch:trusty/postgresql-42",
		"local:trusty/magic-2",
		"local:trusty/magic-10",
	}
	c.Assert(downloader.curls, jc.DeepEquals, expectedCurls)
	c.Assert(uploader.curls, jc.DeepEquals, expectedCurls)

	expectedRefs := []string{
		"postgresql-a77196f",
		"magic-d348864",
		"magic-5f44d22",
	}
	c.Assert(uploader.charmRefs, jc.DeepEquals, expectedRefs)

	c.Assert(downloader.uris, jc.SameContents, []string{
		"/tools/0",
		"/tools/1",
	})
	c.Assert(uploader.tools, jc.DeepEquals, toolsMap)

	c.Assert(downloader.resources, jc.SameContents, []string{
		"app0/blob0",
		"app1/blob1",
	})
	c.Assert(uploader.resources, jc.DeepEquals, map[string]string{
		"app0/blob0": "blob0",
		"app1/blob1": "blob1",
	})
	c.Assert(uploader.unitResources, jc.SameContents, []string{"app1/99-blob1"})
}

func (s *ImportSuite) TestWrongCharmURLAssigned(c *gc.C) {
	downloader := &fakeDownloader{}
	uploader := &fakeUploader{
		reassignCharmURL: true,
	}

	config := migration.UploadBinariesConfig{
		Charms:             []string{"local:foo/bar-2"},
		CharmDownloader:    downloader,
		CharmUploader:      uploader,
		ToolsDownloader:    downloader,
		ToolsUploader:      uploader,
		ResourceDownloader: downloader,
		ResourceUploader:   uploader,
	}
	err := migration.UploadBinaries(config)
	c.Assert(err, gc.ErrorMatches,
		"cannot upload charms: charm local:foo/bar-2 unexpectedly assigned local:foo/bar-100")
}

type fakeDownloader struct {
	curls     []string
	uris      []string
	resources []string
}

func (d *fakeDownloader) OpenCharm(curl string) (io.ReadCloser, error) {
	d.curls = append(d.curls, curl)
	// Return the charm URL string as the fake charm content
	return io.NopCloser(bytes.NewReader([]byte(curl + " content"))), nil
}

func (d *fakeDownloader) OpenURI(uri string, query url.Values) (io.ReadCloser, error) {
	if query != nil {
		panic("query should be empty")
	}
	d.uris = append(d.uris, uri)
	// Return the URI string as fake content
	return io.NopCloser(bytes.NewReader([]byte(uri))), nil
}

func (d *fakeDownloader) OpenResource(app, name string) (io.ReadCloser, error) {
	d.resources = append(d.resources, app+"/"+name)
	// Use the resource name as the content.
	return io.NopCloser(bytes.NewReader([]byte(name))), nil
}

type fakeUploader struct {
	tools            map[version.Binary]string
	curls            []string
	charmRefs        []string
	resources        map[string]string
	unitResources    []string
	reassignCharmURL bool
}

func (f *fakeUploader) UploadTools(r io.ReadSeeker, v version.Binary) (tools.List, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	f.tools[v] = string(data)
	return tools.List{&tools.Tools{Version: v}}, nil
}

func (f *fakeUploader) UploadCharm(curl string, charmRef string, r io.ReadSeeker) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", errors.Trace(err)
	}
	if string(data) != curl+" content" {
		panic(fmt.Sprintf("unexpected charm body for %s: %s", curl, data))
	}
	f.curls = append(f.curls, curl)
	f.charmRefs = append(f.charmRefs, charmRef)

	outU := curl
	if f.reassignCharmURL {
		outU = charm.MustParseURL(outU).WithRevision(100).String()
	}
	return outU, nil
}

func (f *fakeUploader) UploadResource(res resources.Resource, r io.ReadSeeker) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return errors.Trace(err)
	}
	f.resources[res.ApplicationID+"/"+res.Name] = string(body)
	return nil
}

func (f *fakeUploader) SetPlaceholderResource(res resources.Resource) error {
	f.resources[res.ApplicationID+"/"+res.Name] = "<placeholder>"
	return nil
}

func (f *fakeUploader) SetUnitResource(unit string, res resources.Resource) error {
	f.unitResources = append(f.unitResources, unit+"-"+res.Name)
	return nil
}

func fakeGetClaimer(string) (leadership.Claimer, error) {
	return &fakeClaimer{}, nil
}

type fakeClaimer struct {
	leadership.Claimer
	stub testing.Stub
}

func (c *fakeClaimer) ClaimLeadership(application, unit string, duration time.Duration) error {
	c.stub.AddCall("ClaimLeadership", application, unit, duration)
	return c.stub.NextErr()
}
