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
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/dummy"
	_ "github.com/juju/juju/provider/lxd"
	"github.com/juju/juju/testing"
)

type restoreSuite struct {
	BaseBackupsSuite
	wrappedCommand cmd.Command
	command        *backups.RestoreCommand
	store          *jujuclient.MemStore
}

var _ = gc.Suite(&restoreSuite{})

func (s *restoreSuite) SetUpTest(c *gc.C) {
	s.BaseBackupsSuite.SetUpTest(c)

	s.store = jujuclient.NewMemStore()
	s.store.Controllers["testing"] = jujuclient.ControllerDetails{
		ControllerUUID: "deadbeef-0bad-400d-8000-5b1d0d06f00d",
		CACert:         testing.CACert,
		Cloud:          "mycloud",
		CloudRegion:    "a-region",
		APIEndpoints:   []string{"10.0.1.1:17777"},
	}
	s.store.CurrentControllerName = "testing"
	s.store.Models["testing"] = &jujuclient.ControllerModels{
		Models: map[string]jujuclient.ModelDetails{
			"current-user/test1": {ModelUUID: "test1-uuid", ModelType: model.IAAS},
		},
		CurrentModel: "test1",
	}

	s.wrappedCommand, s.command = backups.NewRestoreCommandForTest(s.store)
}

func (s *restoreSuite) patch(c *gc.C, err1, err2 error) (*gomock.Controller, *MockAPIClient, *MockArchiveReader) {
	ctrl := gomock.NewController(c)
	apiClient := NewMockAPIClient(ctrl)
	s.PatchValue(backups.NewAPIClient,
		func(c *backups.CommandBase) (backups.APIClient, error) {
			return apiClient, err1
		},
	)
	archiveClient := NewMockArchiveReader(ctrl)
	s.PatchValue(backups.GetArchive,
		func(string) (backups.ArchiveReader, *params.BackupsMetadataResult, error) {
			return archiveClient, &params.BackupsMetadataResult{}, err2
		},
	)
	return ctrl, apiClient, archiveClient
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
	ctlr, apiClient, archiveReader := s.patch(c, nil, nil)
	defer ctlr.Finish()
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
	ctlr, apiClient, archiveReader := s.patch(c, nil, nil)
	defer ctlr.Finish()
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
	ctlr, apiClient, _ := s.patch(c, nil, nil)
	defer ctlr.Finish()
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
	ctlr, apiClient, _ := s.patch(c, nil, nil)
	defer ctlr.Finish()
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
	ctlr, _, _ := s.patch(c, nil, errors.New("get archive fail"))
	defer ctlr.Finish()
	_, err := cmdtesting.RunCommand(c, s.wrappedCommand, "restore", "--file", "afile")
	c.Assert(err, gc.ErrorMatches, "get archive fail")
}
