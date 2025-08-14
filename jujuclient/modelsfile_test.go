// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type ModelsFileSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func TestModelsFileSuite(t *stdtesting.T) {
	tc.Run(t, &ModelsFileSuite{})
}

const testModelsYAML = `
controllers:
  ctrl:
    models:
      admin/admin:
        uuid: ghi
        type: iaas
  kontroll:
    models:
      admin/admin:
        uuid: abc
        type: iaas
      admin/my-model:
        uuid: def
        type: iaas
    current-model: admin/my-model
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
	ModelUUID: "abc",
	ModelType: model.IAAS,
}
var kontrollMyModelModelDetails = jujuclient.ModelDetails{
	ModelUUID: "def",
	ModelType: model.IAAS,
}
var ctrlAdminModelDetails = jujuclient.ModelDetails{
	ModelUUID: "ghi",
	ModelType: model.IAAS,
}

func (s *ModelsFileSuite) TestWriteFile(c *tc.C) {
	writeTestModelsFile(c)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("models.yaml"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, testModelsYAML[1:])
}

func (s *ModelsFileSuite) TestReadNoFile(c *tc.C) {
	models, err := jujuclient.ReadModelsFile("nowhere.yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.IsNil)
}

func (s *ModelsFileSuite) TestReadEmptyFile(c *tc.C) {
	err := os.WriteFile(osenv.JujuXDGDataHomePath("models.yaml"), []byte(""), 0600)
	c.Assert(err, tc.ErrorIsNil)
	models, err := jujuclient.ReadModelsFile(jujuclient.JujuModelsPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.HasLen, 0)
}

func writeTestModelsFile(c *tc.C) {
	err := jujuclient.WriteModelsFile(testControllerModels)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ModelsFileSuite) TestParseModels(c *tc.C) {
	models, err := jujuclient.ParseModels([]byte(testModelsYAML))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, testControllerModels)
}

const testLegacyModelsYAML = `
controllers:
  ctrl:
    models:
      admin/amodel:
        uuid: ghi
        type: iaas
  kontroll:
    models:
      admin@external/admin:
        uuid: abc
        type: iaas
      admin@external/my-model:
        uuid: def
        type: iaas
    current-model: admin@external/my-model
    previous-model: admin@external/admin
`

var transformedControllerModels = map[string]*jujuclient.ControllerModels{
	"kontroll": {
		Models: map[string]jujuclient.ModelDetails{
			"admin-external/admin":    kontrollAdminModelDetails,
			"admin-external/my-model": kontrollMyModelModelDetails,
		},
		CurrentModel:  "admin-external/my-model",
		PreviousModel: "admin-external/admin",
	},
	"ctrl": {
		Models: map[string]jujuclient.ModelDetails{
			"admin/amodel": ctrlAdminModelDetails,
		},
	},
}

func (s *ModelsFileSuite) TestParseModelsTransformsUserNames(c *tc.C) {
	models, err := jujuclient.ParseModels([]byte(testLegacyModelsYAML))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(models, tc.DeepEquals, transformedControllerModels)
}

func (s *ModelsFileSuite) TestParseModelMetadataError(c *tc.C) {
	models, err := jujuclient.ParseModels([]byte("fail me now"))
	c.Assert(err, tc.ErrorMatches,
		"cannot unmarshal models: yaml: unmarshal errors:"+
			"\n  line 1: cannot unmarshal !!str `fail me...` into "+
			"jujuclient.modelsCollection",
	)
	c.Assert(models, tc.IsNil)
}
