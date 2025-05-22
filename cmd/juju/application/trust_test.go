// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/application/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
)

type TrustSuite struct {
	applicationAPI *mocks.MockApplicationAPI
	store          *jujuclient.MemStore
}

func (s *TrustSuite) SetUpTest(c *tc.C) {
	s.store = jujuclienttesting.MinimalStore()
}
func TestTrustSuite(t *stdtesting.T) {
	tc.Run(t, &TrustSuite{})
}

func (s *TrustSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.applicationAPI = mocks.NewMockApplicationAPI(ctrl)
	return ctrl
}

func (s *TrustSuite) runTrust(c *tc.C, args ...string) error {
	trustCmd := modelcmd.Wrap(&trustCommand{api: s.applicationAPI})
	trustCmd.SetClientStore(s.store)
	_, err := cmdtesting.RunCommand(c, trustCmd, args...)
	return err
}

func (s *TrustSuite) TestTrust(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationAPI.EXPECT().SetConfig(gomock.Any(), "gitlab", "", map[string]string{"trust": "true"})
	s.applicationAPI.EXPECT().Close()

	err := s.runTrust(c, "gitlab")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TrustSuite) TestTrustCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}

	s.applicationAPI.EXPECT().SetConfig(gomock.Any(), "gitlab", "", map[string]string{"trust": "true"})
	s.applicationAPI.EXPECT().Close()

	err := s.runTrust(c, "gitlab", "--scope", "cluster")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TrustSuite) TestTrustCAASRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}

	s.applicationAPI.EXPECT().SetConfig(gomock.Any(), "gitlab", "", map[string]string{"trust": "false"})
	s.applicationAPI.EXPECT().Close()

	err := s.runTrust(c, "gitlab", "--remove")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *TrustSuite) TestTrustCAASNoScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	err := s.runTrust(c, "gitlab")
	c.Assert(err, tc.ErrorMatches, `
'juju trust' currently grants full access to the cluster itself.
Set the scope to 'cluster' using '--scope=cluster' to confirm this choice.
`[1:])
}

func (s *TrustSuite) TestTrustCAASWrongScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.store.Models["arthur"] = &jujuclient.ControllerModels{
		CurrentModel: "king/sword",
		Models: map[string]jujuclient.ModelDetails{"king/sword": {
			ModelType: model.CAAS,
		}},
	}
	err := s.runTrust(c, "gitlab", "--scope", "foo")
	c.Assert(err, tc.ErrorMatches, `scope "foo" not valid`)
}
