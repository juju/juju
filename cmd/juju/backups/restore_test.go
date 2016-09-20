// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"io"
	"sort"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apibackups "github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/network"
	_ "github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type restoreSuite struct {
	BaseBackupsSuite
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	clouds := map[string]cloud.Cloud{
		"mycloud": {
			Type:      "openstack",
			AuthTypes: []cloud.AuthType{"userpass", "access-key"},
			Endpoint:  "http://homestack",
			Regions: []cloud.Region{
				{Name: "a-region", Endpoint: "http://london/1.0"},
			},
		},
	}
	err := cloud.WritePersonalCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)

	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{
		ControllerUUID:         "deadbeef-0bad-400d-8000-5b1d0d06f00d",
		CACert:                 testing.CACert,
		Cloud:                  "mycloud",
		CloudRegion:            "a-region",
		APIEndpoints:           []string{"10.0.1.1:17777"},
		UnresolvedAPIEndpoints: []string{"10.0.1.1:17777"},
	}
	s.store.CurrentControllerName = "testing"
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"admin": {"test1-uuid"},
		},
		CurrentModel: "admin",
	}
	s.store.Accounts["testing"] = jujuclient.AccountDetails{
		User:     "current-user@local",
		Password: "old-password",
	}
	s.store.BootstrapConfig["testing"] = jujuclient.BootstrapConfig{
		Cloud:       "mycloud",
		CloudType:   "dummy",
		CloudRegion: "a-region",
		Config: map[string]interface{}{
			"type": "dummy",
			"name": "admin",
		},
		ControllerModelUUID: testing.ModelTag.Id(),
		ControllerConfig: controller.Config{
			"api-port":   17070,
			"state-port": 37017,
		},
	}
	s.store.Credentials["dummy"] = cloud.CloudCredential{
		AuthCredentials: map[string]cloud.Credential{
			"me": cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
				"username": "user",
				"password": "sekret",
			}),
		},
	}
}

func (s *restoreSuite) TestRestoreArgs(c *gc.C) {
	s.command = backups.NewRestoreCommandForTest(s.store, nil, nil, nil, nil)
	_, err := testing.RunCommand(c, s.command, "restore")
	c.Assert(err, gc.ErrorMatches, "you must specify either a file or a backup id.")

	_, err = testing.RunCommand(c, s.command, "restore", "--id", "anid", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "you must specify either a file or a backup id but not both.")

	_, err = testing.RunCommand(c, s.command, "restore", "--id", "anid", "-b")
	c.Assert(err, gc.ErrorMatches, "it is not possible to rebootstrap and restore from an id.")
}

// TODO(wallyworld) - add more api related unit tests
type mockRestoreAPI struct {
	backups.RestoreAPI
}

func (*mockRestoreAPI) Close() error {
	return nil
}

func (*mockRestoreAPI) RestoreReader(io.ReadSeeker, *params.BackupsMetadataResult, apibackups.ClientConnection) error {
	return nil
}

type mockArchiveReader struct {
	backups.ArchiveReader
}

func (*mockArchiveReader) Close() error {
	return nil
}

func (s *restoreSuite) TestRestoreReboostrapControllerExists(c *gc.C) {
	fakeEnv := fakeEnviron{controllerInstances: []instance.Id{"1"}}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &params.BackupsMetadataResult{}, nil
		},
		backups.GetEnvironFunc(fakeEnv),
		backups.GetRebootstrapParamsFunc("mycloud"),
	)
	_, err := testing.RunCommand(c, s.command, "restore", "--file", "afile", "-b")
	c.Assert(err, gc.ErrorMatches, ".*still seems to exist.*")
}

func (s *restoreSuite) TestRestoreReboostrapNoControllers(c *gc.C) {
	fakeEnv := fakeEnviron{}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &params.BackupsMetadataResult{
				CACert: testing.CACert,
			}, nil
		},
		backups.GetEnvironFunc(fakeEnv),
		backups.GetRebootstrapParamsFunc("mycloud"),
	)
	s.PatchValue(&backups.BootstrapFunc, func(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error {
		return errors.New("failed to bootstrap new controller")
	})

	_, err := testing.RunCommand(c, s.command, "restore", "--file", "afile", "-b")
	c.Assert(err, gc.ErrorMatches, ".*failed to bootstrap new controller")
}

func (s *restoreSuite) TestRestoreReboostrapReadsMetadata(c *gc.C) {
	metadata := params.BackupsMetadataResult{
		CACert:       testing.CACert,
		CAPrivateKey: testing.CAKey,
	}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &metadata, nil
		},
		backups.GetEnvironFunc(fakeEnviron{}),
		backups.GetRebootstrapParamsFunc("mycloud"),
	)
	s.PatchValue(&backups.BootstrapFunc, func(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error {
		return errors.New("failed to bootstrap new controller")
	})

	_, err := testing.RunCommand(c, s.command, "restore", "-m", "testing:test1", "--file", "afile", "-b")
	c.Assert(err, gc.ErrorMatches, ".*failed to bootstrap new controller")
}

func (s *restoreSuite) TestFailedRestoreReboostrapMaintainsControllerInfo(c *gc.C) {
	metadata := params.BackupsMetadataResult{
		CACert:       testing.CACert,
		CAPrivateKey: testing.CAKey,
	}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &metadata, nil
		},
		nil,
		backups.GetRebootstrapParamsFuncWithError(),
	)
	s.PatchValue(&backups.BootstrapFunc, func(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error {
		// We should not call bootstrap.
		c.Fail()
		return nil
	})

	_, err := testing.RunCommand(c, s.command, "restore", "-m", "testing:test1", "--file", "afile", "-b")
	c.Assert(err, gc.ErrorMatches, "failed")
	// The details below are as per what was done in test setup, so no changes.
	c.Assert(s.store.Controllers["testing"], jc.DeepEquals, jujuclient.ControllerDetails{
		Cloud:                  "mycloud",
		CloudRegion:            "a-region",
		CACert:                 testing.CACert,
		ControllerUUID:         "deadbeef-0bad-400d-8000-5b1d0d06f00d",
		APIEndpoints:           []string{"10.0.1.1:17777"},
		UnresolvedAPIEndpoints: []string{"10.0.1.1:17777"},
	})
}

func (s *restoreSuite) TestRestoreReboostrapWritesUpdatedControllerInfo(c *gc.C) {
	metadata := params.BackupsMetadataResult{
		CACert:       testing.CACert,
		CAPrivateKey: testing.CAKey,
	}
	fakeEnv := fakeEnviron{}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &metadata, nil
		},
		backups.GetEnvironFunc(fakeEnv),
		backups.GetRebootstrapParamsFunc("mycloud"),
	)
	boostrapped := false
	s.PatchValue(&backups.BootstrapFunc, func(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error {
		c.Assert(args.ControllerConfig, jc.DeepEquals, controller.Config{
			"controller-uuid":         "deadbeef-1bad-500d-9000-4b1d0d06f00d",
			"ca-cert":                 testing.CACert,
			"state-port":              1234,
			"api-port":                17777,
			"set-numa-control-policy": false,
		})
		boostrapped = true
		return nil
	})

	intPtr := func(i int) *int {
		return &i
	}

	_, err := testing.RunCommand(c, s.command, "restore", "-m", "testing:test1", "--file", "afile", "-b")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(boostrapped, jc.IsTrue)
	c.Assert(s.store.Controllers["testing"], jc.DeepEquals, jujuclient.ControllerDetails{
		Cloud:                  "mycloud",
		CloudRegion:            "a-region",
		CACert:                 testing.CACert,
		ControllerUUID:         "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		APIEndpoints:           []string{"10.0.0.1:17777"},
		UnresolvedAPIEndpoints: []string{"10.0.0.1:17777"},
		AgentVersion:           version.Current.String(),
		ModelCount:             intPtr(2),
		MachineCount:           intPtr(1),
		ControllerMachineCount: 1,
	})
}

func (s *restoreSuite) TestRestoreReboostrapBuiltInProvider(c *gc.C) {
	metadata := params.BackupsMetadataResult{
		CACert:       testing.CACert,
		CAPrivateKey: testing.CAKey,
	}
	fakeEnv := fakeEnviron{}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &metadata, nil
		},
		backups.GetEnvironFunc(fakeEnv),
		backups.GetRebootstrapParamsFunc("lxd"),
	)
	boostrapped := false
	s.PatchValue(&backups.BootstrapFunc, func(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error {
		boostrapped = true
		sort.Sort(args.Cloud.AuthTypes)
		c.Assert(args.Cloud, jc.DeepEquals, cloud.Cloud{
			Type:      "lxd",
			AuthTypes: []cloud.AuthType{"empty"},
			Regions:   []cloud.Region{{Name: "localhost"}},
		})
		return nil
	})

	_, err := testing.RunCommand(c, s.command, "restore", "-m", "testing:test1", "--file", "afile", "-b")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(boostrapped, jc.IsTrue)
}

type fakeInstance struct {
	instance.Instance
	id instance.Id
}

func (f fakeInstance) Addresses() ([]network.Address, error) {
	return []network.Address{
		{Value: "10.0.0.1"},
	}, nil
}

type fakeEnviron struct {
	environs.Environ
	controllerInstances []instance.Id
}

func (f fakeEnviron) ControllerInstances(_ string) ([]instance.Id, error) {
	return f.controllerInstances, nil
}

func (f fakeEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return []instance.Instance{fakeInstance{id: "1"}}, nil
}

func (f fakeEnviron) AllInstances() ([]instance.Instance, error) {
	return []instance.Instance{fakeInstance{id: "1"}}, nil
}
