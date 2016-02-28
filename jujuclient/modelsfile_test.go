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
    accounts:
      admin@local:
        models:
          admin:
            uuid: ghi
      bob@local:
        models:
          admin:
            uuid: jkl
  kontroll:
    accounts:
      admin@local:
        models:
          admin:
            uuid: abc
          my-model:
            uuid: def
        current-model: my-model
`

var testControllerModels = map[string]jujuclient.ControllerAccountModels{
	"kontroll": {
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{
					"admin":    kontrollAdminModelDetails,
					"my-model": kontrollMyModelModelDetails,
				},
				CurrentModel: "my-model",
			},
		},
	},
	"ctrl": {
		map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{
					"admin": ctrlAdminAdminModelDetails,
				},
			},
			"bob@local": {
				Models: map[string]jujuclient.ModelDetails{
					"admin": ctrlBobAdminModelDetails,
				},
			},
		},
	},
}

var kontrollAdminModelDetails = jujuclient.ModelDetails{"abc"}
var kontrollMyModelModelDetails = jujuclient.ModelDetails{"def"}
var ctrlAdminAdminModelDetails = jujuclient.ModelDetails{"ghi"}
var ctrlBobAdminModelDetails = jujuclient.ModelDetails{"jkl"}

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
