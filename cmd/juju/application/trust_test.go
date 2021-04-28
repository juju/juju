// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type TrustSuite struct {
	applicationAPI *mocks.MockApplicationAPI
	store          *jujuclient.MemStore
}

func (s *TrustSuite) SetUpTest(c *gc.C) {
	s.store = jujuclienttesting.MinimalStore()
}

var _ = gc.Suite(&TrustSuite{})

func (s *TrustSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	return ctrl
}

func (s *TrustSuite) runTrust(c *gc.C, args ...string) error {
	configCmd := configCommand{api: s.applicationAPI}
	configCmd.SetClientStore(s.store)
	trustCmd := modelcmd.Wrap(&trustCommand{configCommand: configCmd})
	_, err := cmdtesting.RunCommand(c, trustCmd, args...)
	return err
}

func (s *TrustSuite) TestTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationAPI.EXPECT().Get("", "gitlab").Return(&params.ApplicationGetResults{}, nil)
	s.applicationAPI.EXPECT().SetConfig("", "gitlab", "", map[string]string{"trust": "true"})
	s.applicationAPI.EXPECT().Close()

	err := s.runTrust(c, "gitlab")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TrustSuite) TestTrustCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}

	s.applicationAPI.EXPECT().Get("", "gitlab").Return(&params.ApplicationGetResults{}, nil)
	s.applicationAPI.EXPECT().SetConfig("", "gitlab", "", map[string]string{"trust": "true"})
	s.applicationAPI.EXPECT().Close()

	err := s.runTrust(c, "gitlab", "--scope", "cluster")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TrustSuite) TestTrustCAASRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}

	s.applicationAPI.EXPECT().Get("", "gitlab").Return(&params.ApplicationGetResults{}, nil)
	s.applicationAPI.EXPECT().SetConfig("", "gitlab", "", map[string]string{"trust": "false"})
	s.applicationAPI.EXPECT().Close()

	err := s.runTrust(c, "gitlab", "--remove")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *TrustSuite) TestTrustCAASNoScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	err := s.runTrust(c, "gitlab")
	c.Assert(err, gc.ErrorMatches, `
'juju trust' currently grants full access to the cluster itself.
Set the scope to 'cluster' using '--scope=cluster' to confirm this choice.
`[1:])
}

func (s *TrustSuite) TestTrustCAASWrongScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	err := s.runTrust(c, "gitlab", "--scope", "foo")
	c.Assert(err, gc.ErrorMatches, `scope "foo" not valid`)
}
