// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type restoreSuite struct {
	BaseBackupsSuite
	store *jujuclienttesting.MemStore
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)
	s.store = jujuclienttesting.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{
		ControllerUUID: testing.ModelTag.Id(),
		CACert:         testing.CACert,
	}
	s.store.Models["testing"] = jujuclient.ControllerAccountModels{
		AccountModels: map[string]*jujuclient.AccountModels{
			"admin@local": {
				Models: map[string]jujuclient.ModelDetails{
					"test1": {"test1-uuid"},
				},
				CurrentModel: "test1",
			},
		},
	}
	s.store.Accounts["testing"] = &jujuclient.ControllerAccounts{
		Accounts: map[string]jujuclient.AccountDetails{
			"admin@local": {
				User:     "current-user@local",
				Password: "old-password",
			},
		},
		CurrentAccount: "admin@local",
	}
	s.store.BootstrapConfig["testing"] = jujuclient.BootstrapConfig{
		Cloud: "dummy",
		//Credential: "me",
		Config: map[string]interface{}{
			"type": "dummy",
			"name": "admin",
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
	s.command = backups.NewRestoreCommandForTest(s.store, nil, nil, nil)
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
		func(string, *params.BackupsMetadataResult) (environs.Environ, error) {
			return fakeEnv, nil
		})
	_, err := testing.RunCommand(c, s.command, "restore", "--file", "afile", "-b")
	c.Assert(err, gc.ErrorMatches, ".*still seems to exist.*")
}

func (s *restoreSuite) TestRestoreReboostrapNoControllers(c *gc.C) {
	fakeEnv := fakeEnviron{}
	s.command = backups.NewRestoreCommandForTest(
		s.store, &mockRestoreAPI{},
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return &mockArchiveReader{}, &params.BackupsMetadataResult{}, nil
		},
		func(string, *params.BackupsMetadataResult) (environs.Environ, error) {
			return fakeEnv, nil
		})
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
		nil)
	s.PatchValue(&backups.BootstrapFunc, func(ctx environs.BootstrapContext, environ environs.Environ, args bootstrap.BootstrapParams) error {
		attr := environ.Config().AllAttrs()
		c.Assert(attr["ca-cert"], gc.Equals, testing.CACert)
		return errors.New("failed to bootstrap new controller")
	})

	_, err := testing.RunCommand(c, s.command, "restore", "-m", "testing:test1", "--file", "afile", "-b")
	c.Assert(err, gc.ErrorMatches, ".*failed to bootstrap new controller")
}

type fakeInstance struct {
	instance.Instance
	id instance.Id
}

type fakeEnviron struct {
	environs.Environ
	controllerInstances []instance.Id
}

func (f fakeEnviron) ControllerInstances() ([]instance.Id, error) {
	return f.controllerInstances, nil
}

func (f fakeEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return []instance.Instance{fakeInstance{id: "1"}}, nil
}
