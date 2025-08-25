// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"os"

	"github.com/canonical/lxd/shared/api"
	lxdapi "github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/context/mocks"
)

type upgradesSuite struct {
	testing.IsolationSuite
	server *MockServer
	ctx    context.ProviderCallContext
}

var _ = gc.Suite(&upgradesSuite{})

func (s *upgradesSuite) TestReadLegacyCloudCredentials(c *gc.C) {
	var paths []string
	readFile := func(path string) ([]byte, error) {
		paths = append(paths, path)
		return []byte("content: " + path), nil
	}
	cred, err := ReadLegacyCloudCredentials(readFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cred, jc.DeepEquals, cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		"client-cert": "content: /etc/juju/lxd-client.crt",
		"client-key":  "content: /etc/juju/lxd-client.key",
		"server-cert": "content: /etc/juju/lxd-server.crt",
	}))
	c.Assert(paths, jc.DeepEquals, []string{
		"/etc/juju/lxd-client.crt",
		"/etc/juju/lxd-client.key",
		"/etc/juju/lxd-server.crt",
	})
}

func (s *upgradesSuite) TestReadLegacyCloudCredentialsFileNotExist(c *gc.C) {
	readFile := func(path string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	_, err := ReadLegacyCloudCredentials(readFile)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *upgradesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.server = NewMockServer(ctrl)
	s.ctx = mocks.NewMockProviderCallContext(ctrl)
	return ctrl
}

func (s *upgradesSuite) newEnviron(c *gc.C) *environ {
	modelUUID := utils.MustNewUUID().String()
	namespace, err := instance.NewNamespace(modelUUID)
	c.Assert(err, gc.IsNil)

	return &environ{
		name:           "model",
		namespace:      namespace,
		uuid:           modelUUID,
		serverUnlocked: s.server,
	}
}

func (s *upgradesSuite) TestDescription(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnviron(c)
	profileStep := createProfilesStep{env: env}

	desc := profileStep.Description()

	c.Assert(desc, gc.Equals, "Create and assign LXD profiles to instances.")
}

// TestRunCreateModelAndCharmProfiles runs a test case to create a model and charm profile.
func (s *upgradesSuite) TestRunCreateModelAndCharmProfiles(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnviron(c)
	modelShortID := names.NewModelTag(env.uuid).ShortId()
	oldModelProfileName := fmt.Sprintf("juju-%s", env.name)
	oldMyCharmProfileName := fmt.Sprintf("juju-%s-mycharm-1", env.name)
	oldMyCharmProfile := &api.Profile{
		Name:        oldMyCharmProfileName,
		Description: "awesome profile",
		Config:      map[string]string{"name": "juju", "year": "2025"},
		Devices:     map[string]map[string]string{"eth0": {"type": "nic"}},
		UsedBy:      []string{"juju-1"},
	}
	newMyCharmProfileName := fmt.Sprintf("juju-%s-%s-mycharm-1", env.name, modelShortID)
	newModelProfileName := fmt.Sprintf("juju-%s-%s", env.name, modelShortID)

	gomock.InOrder(
		s.server.EXPECT().HasProfile(env.profileName()).Return(false, nil),
		// re-create the model profile
		s.server.EXPECT().CreateProfileWithConfig(env.profileName(), map[string]string{
			"boot.autostart":   "true",
			"security.nesting": "true",
		}).Return(nil),
		s.server.EXPECT().AliveContainers(env.namespace.Prefix()).Return([]lxd.Container{
			{
				api.Instance{
					Name: "juju-1",
				}},
			{
				api.Instance{
					Name: "juju-2",
				}},
		}, nil),
		// Get profiles for instance "juju-1"
		s.server.EXPECT().GetContainerProfiles("juju-1").Return([]string{
			"default",
			// model profile -- this one is skipped.
			oldModelProfileName,
			// charm profile -- this one is what we want because
			// we want to re-create the charm profile.
			oldMyCharmProfileName,
		}, nil),
		s.server.EXPECT().GetProfile(oldMyCharmProfileName).Return(oldMyCharmProfile, "", nil),

		// Try to create the new charm profile
		s.server.EXPECT().HasProfile(newMyCharmProfileName).Return(false, nil),
		s.server.EXPECT().CreateProfile(lxdapi.ProfilesPost{
			ProfilePut: lxdapi.ProfilePut{
				Config:      oldMyCharmProfile.Config,
				Description: oldMyCharmProfile.Description,
				Devices:     oldMyCharmProfile.Devices,
			},
			Name: newMyCharmProfileName,
		}).Return(nil),

		// Get profiles for instance "juju-2".
		// This instance doesn't have a charms profile, so we don't create new ones for it.
		s.server.EXPECT().GetContainerProfiles("juju-2").Return([]string{"default", fmt.Sprintf("juju-%s", env.name)}, nil),

		// Attach the new profiles for instance "juju-1"
		s.server.EXPECT().GetContainerProfiles("juju-1").Return([]string{"default", oldModelProfileName, oldMyCharmProfileName}, nil),
		s.server.EXPECT().UpdateContainerProfiles("juju-1", []string{"default", newModelProfileName, newMyCharmProfileName}).Return(nil),

		// Attach the new profiles for instance "juju-2"
		s.server.EXPECT().GetContainerProfiles("juju-2").Return([]string{"default", oldModelProfileName}, nil),
		s.server.EXPECT().UpdateContainerProfiles("juju-2", []string{"default", newModelProfileName}).Return(nil),

		// Delete the old profiles for instance "juju-1"
		s.server.EXPECT().DeleteProfile(oldModelProfileName).Return(nil),
		// Doesn't crash the worker
		s.server.EXPECT().DeleteProfile(oldMyCharmProfileName).Return(fmt.Errorf("used by an instance")),
	)

	profileStep := createProfilesStep{env: env}

	err := profileStep.Run(s.ctx)

	c.Assert(err, gc.IsNil)
}

// TestRunCreateCharmProfiles runs a test case where a new model profile already exists (and is attached) so we just create the charm profiles.
func (s *upgradesSuite) TestRunCreateCharmProfiles(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnviron(c)
	modelShortID := names.NewModelTag(env.uuid).ShortId()
	oldModelProfileName := fmt.Sprintf("juju-%s", env.name)
	oldMyCharmProfileName := fmt.Sprintf("juju-%s-mycharm-1", env.name)
	oldMyCharmProfile := &api.Profile{
		Name:        oldMyCharmProfileName,
		Description: "awesome profile",
		Config:      map[string]string{"name": "juju", "year": "2025"},
		Devices:     map[string]map[string]string{"eth0": {"type": "nic"}},
		UsedBy:      []string{"juju-1"},
	}
	newMyCharmProfileName := fmt.Sprintf("juju-%s-%s-mycharm-1", env.name, modelShortID)
	newModelProfileName := fmt.Sprintf("juju-%s-%s", env.name, modelShortID)

	gomock.InOrder(
		// New model profile exists so we don't create a new one.
		s.server.EXPECT().HasProfile(env.profileName()).Return(true, nil),

		s.server.EXPECT().AliveContainers(env.namespace.Prefix()).Return([]lxd.Container{
			{
				api.Instance{
					Name: "juju-1",
				}},
			{
				api.Instance{
					Name: "juju-2",
				}},
		}, nil),

		// Get profiles for instance "juju-1"
		s.server.EXPECT().GetContainerProfiles("juju-1").Return([]string{
			"default",
			// model profile -- this one is skipped.
			oldModelProfileName,
			// charm profile -- this one is what we want because
			// we want to re-create the charm profile.
			oldMyCharmProfileName,
			// new model profile -- must've been created and attached on a previous worker run.
			newModelProfileName,
		}, nil),
		s.server.EXPECT().GetProfile(oldMyCharmProfileName).Return(oldMyCharmProfile, "", nil),

		// Try to create the new charm profile
		s.server.EXPECT().HasProfile(newMyCharmProfileName).Return(false, nil),
		s.server.EXPECT().CreateProfile(lxdapi.ProfilesPost{
			ProfilePut: lxdapi.ProfilePut{
				Config:      oldMyCharmProfile.Config,
				Description: oldMyCharmProfile.Description,
				Devices:     oldMyCharmProfile.Devices,
			},
			Name: newMyCharmProfileName,
		}).Return(nil),

		// Get profiles for instance "juju-2".
		// This instance doesn't have a charms profile, so we don't create new ones for it.
		s.server.EXPECT().GetContainerProfiles("juju-2").Return([]string{"default", fmt.Sprintf("juju-%s", env.name)}, nil),

		// Attach the new profiles for instance "juju-1"
		s.server.EXPECT().GetContainerProfiles("juju-1").Return([]string{"default", oldModelProfileName, oldMyCharmProfileName}, nil),
		// Despite the new model profile already attached, we re-attach it here. The LXD API treats this as a safe operation.
		s.server.EXPECT().UpdateContainerProfiles("juju-1", []string{"default", newModelProfileName, newMyCharmProfileName}).Return(nil),

		// Attach the new profiles for instance "juju-2"
		s.server.EXPECT().GetContainerProfiles("juju-2").Return([]string{"default", oldModelProfileName}, nil),
		s.server.EXPECT().UpdateContainerProfiles("juju-2", []string{"default", newModelProfileName}).Return(nil),

		// Delete the old profiles for instance "juju-1"
		s.server.EXPECT().DeleteProfile(oldModelProfileName).Return(nil),
		// Doesn't crash the worker
		s.server.EXPECT().DeleteProfile(oldMyCharmProfileName).Return(fmt.Errorf("used by an instance")),
	)

	profileStep := createProfilesStep{env: env}

	err := profileStep.Run(s.ctx)

	c.Assert(err, gc.IsNil)
}

func (s *upgradesSuite) TestRunErrorHasProfile(c *gc.C) {
	defer s.setupMocks(c).Finish()
	env := s.newEnviron(c)

	s.server.EXPECT().HasProfile(env.profileName()).Return(false, fmt.Errorf("connection issue"))

	profileStep := createProfilesStep{env: env}

	err := profileStep.Run(s.ctx)

	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("init profile %q: connection issue", env.profileName()))
}
