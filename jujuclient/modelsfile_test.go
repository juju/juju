// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/featureflag"
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
        type: iaas
        branch: master
  kontroll:
    models:
      admin/admin:
        uuid: abc
        type: iaas
        branch: master
      admin/my-model:
        uuid: def
        type: iaas
        branch: master
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

var kontrollAdminModelDetails = jujuclient.ModelDetails{
	ModelUUID:    "abc",
	ModelType:    model.IAAS,
	ActiveBranch: model.GenerationMaster,
}
var kontrollMyModelModelDetails = jujuclient.ModelDetails{
	ModelUUID:    "def",
	ModelType:    model.IAAS,
	ActiveBranch: model.GenerationMaster,
}
var ctrlAdminModelDetails = jujuclient.ModelDetails{
	ModelUUID:    "ghi",
	ModelType:    model.IAAS,
	ActiveBranch: model.GenerationMaster,
}

func (s *ModelsFileSuite) TestWriteFile(c *gc.C) {
	writeTestModelsFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("models.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, testModelsYAML[1:])
}

func (s *ModelsFileSuite) TestReadNoFile(c *gc.C) {
	models, err := jujuclient.ReadModelsFile("nowhere.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.IsNil)
}

func (s *ModelsFileSuite) TestReadEmptyFile(c *gc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("models.yaml"), []byte(""), 0600)
	c.Assert(err, jc.ErrorIsNil)
	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(models, gc.HasLen, 0)
}

func (s *ModelsFileSuite) TestMigrateLegacyLocal(c *gc.C) {
	s.SetFeatureFlags(featureflag.Branches)
	err := os.WriteFile(jujuclient.JujuModelsPath(), []byte(testLegacyModelsYAML), 0644)
	c.Assert(err, jc.ErrorIsNil)

	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)

	migratedData, err := os.ReadFile(jujuclient.JujuModelsPath())
	c.Assert(err, jc.ErrorIsNil)
	migratedModels, err := jujuclient.ParseModels(migratedData)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(migratedData), jc.DeepEquals, testModelsYAML[1:])
	c.Assert(migratedModels, jc.DeepEquals, models)
}

func writeTestModelsFile(c *gc.C) {
	err := jujuclient.WriteModelsFile(testControllerModels)
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
