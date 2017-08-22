// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type ModelsFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&ModelsFileSuite{})

const testModelsYAML = `
controllers:
  ctrl:
    models:
      admin/admin:
        uuid: ghi
  kontroll:
    models:
      admin/admin:
        uuid: abc
      admin/my-model:
        uuid: def
    current-model: admin/my-model
`

const testLegacyModelsYAML = `
controllers:
  ctrl:
    models:
      admin@local/admin:
        uuid: ghi
  kontroll:
    models:
      admin@local/admin:
        uuid: abc
      admin@local/my-model:
        uuid: def
    current-model: admin@local/my-model
`

var testControllerModels = map[string]*jujuclient.ControllerModels{
	"kontroll": {
		Models: map[string]jujuclient.ModelDetails{
			"admin/admin":    kontrollAdminModelDetails,
			"admin/my-model": kontrollMyModelModelDetails,
		},
		CurrentModel: "admin/my-model",
	},
	"ctrl": {
		Models: map[string]jujuclient.ModelDetails{
			"admin/admin": ctrlAdminModelDetails,
		},
	},
}

const testControllerModelsYaml = `
controllers:
  ctrl:
    uuid: this-is-the-ctrl-test-uuid
    model-count: 1
  kontroll:
    uuid: this-is-kontroll-uuid
    model-count: 2
`

var kontrollAdminModelDetails = jujuclient.ModelDetails{"abc"}
var kontrollMyModelModelDetails = jujuclient.ModelDetails{"def"}
var ctrlAdminModelDetails = jujuclient.ModelDetails{"ghi"}

func (s *ModelsFileSuite) TestWriteFile(c *gc.C) {
	writeTestModelsFile(c)
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("models.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testModelsYAML[1:])
}

func (s *ModelsFileSuite) TestReadNoFile(c *gc.C) {
	models, err := jujuclient.ReadModelsFile("nowhere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.IsNil)
}

func (s *ModelsFileSuite) TestReadEmptyFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("models.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)
	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelsFileSuite) TestMigrateLegacyLocal(c *gc.C) {
	err := ioutil.WriteFile(jujuclient.JujuModelsPath(), []byte(testLegacyModelsYAML), 0644)
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)

	migratedData, err := ioutil.ReadFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	migratedModels, err := jujuclient.ParseModels(migratedData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(migratedData), jc.DeepEquals, testModelsYAML[1:])
	c.Assert(migratedModels, jc.DeepEquals, models)
}

func writeTestModelsFile(c *gc.C) {
	err := jujuclient.WriteModelsFile(testControllerModels)
	c.Assert(err, jc.ErrorIsNil)

	// we also need corresponding controllers file since
	// some model operations will affect stored controllers data.
	controllers, err := jujuclient.ParseControllers([]byte(testControllerModelsYaml))
	c.Assert(err, jc.ErrorIsNil)
	err = jujuclient.WriteControllersFile(controllers)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelsFileSuite) TestParseModels(c *gc.C) {
	models, err := jujuclient.ParseModels([]byte(testModelsYAML))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, jc.DeepEquals, testControllerModels)
}

func (s *ModelsFileSuite) TestParseModelMetadataError(c *gc.C) {
	models, err := jujuclient.ParseModels([]byte("fail me now"))
	c.Assert(err, gc.ErrorMatches,
		"cannot unmarshal models: yaml: unmarshal errors:"+
			"\n  line 1: cannot unmarshal !!str `fail me...` into "+
			"jujuclient.modelsCollection",
	)
	c.Assert(models, gc.IsNil)
}
