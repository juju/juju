// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	coremodel "github.com/juju/juju/core/model"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/model"
	"github.com/juju/juju/cmd/juju/model/mocks"
)

type showGenerationSuite struct {
	generationBaseSuite

	api *mocks.MockShowGenerationCommandAPI
}

var _ = gc.Suite(&showGenerationSuite{})

func (s *showGenerationSuite) runInit(args ...string) error {
	cmd := model.NewCancelGenerationCommandForTest(nil, s.store)
	return cmdtesting.InitCommand(cmd, args)
}

func (s *showGenerationSuite) TestInit(c *gc.C) {
	err := s.runInit()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *showGenerationSuite) TestInitFail(c *gc.C) {
	err := s.runInit("test")
	c.Assert(err, gc.ErrorMatches, "No arguments allowed")
}

func (s *showGenerationSuite) TestRunCommandNextGenExists(c *gc.C) {
	defer s.setup(c).Finish()

	result := map[coremodel.GenerationVersion][]coremodel.GenerationApplication{
		coremodel.GenerationNext: {
			coremodel.GenerationApplication{
				ApplicationName: "redis",
				Units:           []string{"redis/0"},
				ConfigChanges:   map[string]interface{}{"databases": 8},
			},
		},
	}
	s.api.EXPECT().GenerationInfo(gomock.Any()).Return(result, nil)

	ctx, err := s.runCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
next:
- application: redis
  units:
  - redis/0
  config:
    databases: 8
`[1:])
}

func (s *showGenerationSuite) TestRunCommandNextNoGenError(c *gc.C) {
	defer s.setup(c).Finish()

	s.api.EXPECT().GenerationInfo(gomock.Any()).Return(nil, errors.New("this model has no next generation"))

	_, err := s.runCommand(c)
	c.Assert(err, gc.ErrorMatches, "this model has no next generation")
}

func (s *showGenerationSuite) runCommand(c *gc.C) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, model.NewShowGenerationCommandForTest(s.api, s.store))
}

func (s *showGenerationSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.api = mocks.NewMockShowGenerationCommandAPI(ctrl)
	s.api.EXPECT().Close()
	return ctrl
}
