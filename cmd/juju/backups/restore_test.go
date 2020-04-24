// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"fmt"
	"path/filepath"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/testing"
)

type restoreSuite struct {
	BaseBackupsSuite
	wrappedCommand cmd.Command
	command        *backups.RestoreCommand
}

var _ = gc.Suite(&restoreSuite{})

const (
	controllerUUID      = "deadbeef-0bad-400d-8000-5b1d0d06f00d"
	controllerModelUUID = "deadbeef-0bad-400d-8000-5b1d0d06f000"
	test1ModelUUID      = "deadbeef-0bad-400d-8000-5b1d0d06f001"
)

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)

	controllerName := "test-master"
	s.store.Controllers[controllerName] = jujuclient.ControllerDetails{
		ControllerUUID: controllerUUID,
		CACert:         testing.CACert,
		Cloud:          "mycloud",
		CloudRegion:    "a-region",
		APIEndpoints:   []string{"10.0.1.1:17777"},
	}
	s.store.CurrentControllerName = controllerName
	s.store.Accounts[controllerName] = jujuclient.AccountDetails{
		User: "bob",
	}
	s.store.Models[controllerName] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"bob/test1":        {ModelUUID: test1ModelUUID, ModelType: model.IAAS},
			"admin/controller": {ModelUUID: controllerModelUUID, ModelType: model.IAAS},
		},
		CurrentModel: "controller",
	}
	s.wrappedCommand, s.command = backups.NewRestoreCommandForTest(s.store)
}

func (s *restoreSuite) patch(c *gc.C, archiveErr error) (*gomock.Controller, *MockAPIClient, *MockArchiveReader, *MockModelStatusAPI) {
	ctrl := gomock.NewController(c)
	apiClient := NewMockAPIClient(ctrl)
	s.PatchValue(backups.NewAPIClient,
		func(*backups.CommandBase) (backups.APIClient, error) {
			return apiClient, nil
		},
	)
	archiveClient := NewMockArchiveReader(ctrl)
	s.PatchValue(backups.GetArchive,
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return archiveClient, &params.BackupsMetadataResult{}, archiveErr
		},
	)
	modelStatusClient := NewMockModelStatusAPI(ctrl)
	s.command.AssignGetModelStatusAPI(
		func() (backups.ModelStatusAPI, error) {
			return modelStatusClient, nil
		},
	)

	return ctrl, apiClient, archiveClient, modelStatusClient
}

// expectModelStatus is a convenience functions for the expectations
// concerning successful ModelStatus based on a non HA config.
func expectModelStatus(modelStatusClient *MockModelStatusAPI) {
	controllerModelTag := names.NewModelTag(controllerModelUUID)
	gomock.InOrder(
		modelStatusClient.EXPECT().ModelStatus(controllerModelTag).Return(
			[]base.ModelStatus{{
				UUID: controllerModelUUID,
				Machines: []base.Machine{
					{HasVote: true, WantsVote: true, Status: string(status.Active)},
				},
			}}, nil,
		),
		modelStatusClient.EXPECT().Close(),
	)
}

type restoreBackupArgParsing struct {
	title    string
	args     []string
	errMatch string
	id       string
	filename string
}

var testRestoreBackupArgParsing = []restoreBackupArgParsing{
	{
		title:    "no args",
		args:     []string{"--id", "anid", "--file", "afile"},
		errMatch: "you must specify either a file or a backup id but not both.",
	},
	{
		title:    "arg mismatch: id and file",
		args:     []string{"--id", "anid", "--file", "afile"},
		errMatch: "you must specify either a file or a backup id but not both.",
	},
	{
		title: "id",
		args:  []string{"--id", "anid"},
		id:    "anid",
	},
	{
		title:    "file",
		args:     []string{"--file", "afile"},
		filename: "afile",
	},
}

func (s *restoreSuite) TestArgParsing(c *gc.C) {
	for i, test := range testRestoreBackupArgParsing {
		c.Logf("%d: %s", i, test.title)
		err := cmdtesting.InitCommand(s.wrappedCommand, test.args)
		if test.errMatch == "" {
			c.Assert(err, jc.ErrorIsNil)
			obtainedName := filepath.Base(s.command.Filename)
			expectedName := filepath.Base(test.filename)
			c.Assert(obtainedName, gc.Equals, expectedName)
			c.Assert(s.command.BackupId, gc.Equals, test.id)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errMatch)
		}
	}
}

func (s *restoreSuite) TestRestoreFromBackupFilename(c *gc.C) {
	ctlr, apiClient, archiveReader, modelStatusClient := s.patch(c, nil)
	defer ctlr.Finish()
	expectModelStatus(modelStatusClient)
	gomock.InOrder(
		apiClient.EXPECT().RestoreReader(archiveReader, &params.BackupsMetadataResult{}, gomock.Any()).Return(
			nil,
		),
		apiClient.EXPECT().Close(),
		archiveReader.EXPECT().Close(),
	)
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--file", "afile")
	c.Assert(err, jc.ErrorIsNil)
	out := fmt.Sprintf("restore from %q completed\n", s.command.Filename)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, out)
}

func (s *restoreSuite) TestRestoreFromBackupFilenameFail(c *gc.C) {
	ctlr, apiClient, archiveReader, modelStatusClient := s.patch(c, nil)
	defer ctlr.Finish()
	expectModelStatus(modelStatusClient)
	gomock.InOrder(
		apiClient.EXPECT().RestoreReader(archiveReader, &params.BackupsMetadataResult{}, gomock.Any()).Return(
			errors.New("restore failed"),
		),
		apiClient.EXPECT().Close(),
		archiveReader.EXPECT().Close(),
	)
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "restore failed")
}

func (s *restoreSuite) TestRestoreFromBackupId(c *gc.C) {
	ctlr, apiClient, _, modelStatusClient := s.patch(c, nil)
	defer ctlr.Finish()
	expectModelStatus(modelStatusClient)
	gomock.InOrder(
		apiClient.EXPECT().Restore("an_id", gomock.Any()).Return(
			nil,
		),
		apiClient.EXPECT().Close(),
	)
	ctx, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--id", "an_id")
	c.Assert(err, jc.ErrorIsNil)
	out := fmt.Sprintf("restore from %q completed\n", s.command.BackupId)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, out)
}

func (s *restoreSuite) TestRestoreFromBackupIdFail(c *gc.C) {
	ctlr, apiClient, _, modelStatusClient := s.patch(c, nil)
	defer ctlr.Finish()
	expectModelStatus(modelStatusClient)
	gomock.InOrder(
		apiClient.EXPECT().Restore("an_id", gomock.Any()).Return(
			errors.New("restore failed"),
		),
		apiClient.EXPECT().Close(),
	)
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--id", "an_id")
	c.Assert(err, gc.ErrorMatches, "restore failed")
}

func (s *restoreSuite) TestRestoreFromBackupGetArchiveFail(c *gc.C) {
	ctlr, _, _, modelStatusClient := s.patch(c, errors.New("get archive fail"))
	defer ctlr.Finish()
	expectModelStatus(modelStatusClient)
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "get archive fail")
}

func (s *restoreSuite) TestRestoreFromBackupGetModelStatusFail(c *gc.C) {
	ctlr, _, _, modelStatusClient := s.patch(c, nil)
	defer ctlr.Finish()
	controllerModelTag := names.NewModelTag(controllerModelUUID)
	gomock.InOrder(
		modelStatusClient.EXPECT().ModelStatus(controllerModelTag).Return(
			[]base.ModelStatus{{
				UUID: "",
				Machines: []base.Machine{
					{},
				},
			}}, errors.New("get model status fail"),
		),
		modelStatusClient.EXPECT().Close(),
	)
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "cannot refresh controller model: get model status fail")
}

func (s *restoreSuite) TestRestoreFromBackupHAFail(c *gc.C) {
	ctlr, _, _, modelStatusClient := s.patch(c, nil)
	defer ctlr.Finish()
	controllerModelTag := names.NewModelTag(controllerModelUUID)
	gomock.InOrder(
		modelStatusClient.EXPECT().ModelStatus(controllerModelTag).Return(
			[]base.ModelStatus{{
				UUID: controllerModelUUID,
				Machines: []base.Machine{
					{HasVote: true, WantsVote: true, Status: string(status.Active)},
					{HasVote: true, WantsVote: true, Status: string(status.Active)},
					{HasVote: true, WantsVote: true, Status: string(status.Active)},
				},
			}}, nil,
		),
		modelStatusClient.EXPECT().Close(),
	)
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--id", "an_id")
	c.Assert(err, gc.ErrorMatches, "unable to restore backup in HA configuration.  For help see https://jaas.ai/docs/controller-backups")
}
