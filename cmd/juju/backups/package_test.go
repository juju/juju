// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	apibackups "github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	jujutesting "github.com/juju/juju/testing"
)

// MetaResultString is the expected output of running dumpMetadata() on
// s.metaresult.
var MetaResultString = `

backup ID:             spam 
backup format version: 0 
juju version:          0.0.0 
series:                 

controller UUID:       
model UUID:             
machine ID:             
created on host:        

checksum:               
checksum format:        
size (B):              0 
stored:                0001-01-01 00:00:00 +0000 UTC 
started:               0001-01-01 00:00:00 +0000 UTC 
finished:              0001-01-01 00:00:00 +0000 UTC 

notes:                  

`[1:]

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseBackupsSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite

	command    cmd.Command
	metaresult *params.BackupsMetadataResult
	data       string
	apiVersion int

	filename string

	store *jujuclient.MemStore
}

func (s *BaseBackupsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.metaresult = &params.BackupsMetadataResult{
		ID:       "spam",
		Filename: "filename",
	}
	s.data = "<compressed archive data>"

	s.apiVersion = 2

	s.store = jujuclienttesting.MinimalStore()
	models := s.store.Models["arthur"]
	models.Models["admin/controller"] = jujuclient.ModelDetails{
		ModelUUID: utils.MustNewUUID().String(),
		ModelType: model.IAAS,
	}
	s.store.Models["arthur"] = models
}

func (s *BaseBackupsSuite) TearDownTest(c *gc.C) {
	if s.filename != "" {
		err := os.Remove(s.filename)
		if !os.IsNotExist(err) {
			c.Check(err, jc.ErrorIsNil)
		}
	}

	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *BaseBackupsSuite) patchAPIClient(client backups.APIClient) {
	s.PatchValue(backups.NewAPIClient,
		func(c *backups.CommandBase) (backups.APIClient, error) {
			return client, nil
		},
	)
}

func (s *BaseBackupsSuite) patchGetAPI(client backups.APIClient) {
	s.PatchValue(backups.NewGetAPI,
		func(c *backups.CommandBase) (backups.APIClient, int, error) {
			return client, s.apiVersion, nil
		},
	)
}

func (s *BaseBackupsSuite) setSuccess() *fakeAPIClient {
	client := &fakeAPIClient{metaresult: s.metaresult}
	s.patchAPIClient(client)
	return client
}

func (s *BaseBackupsSuite) setFailure(failure string) *fakeAPIClient {
	client := &fakeAPIClient{err: errors.New(failure)}
	s.patchAPIClient(client)
	return client
}

func (s *BaseBackupsSuite) setDownload() *fakeAPIClient {
	client := s.setSuccess()
	client.archive = ioutil.NopCloser(bytes.NewBufferString(s.data))
	return client
}

func (s *BaseBackupsSuite) createCommandForGlobalOptionTesting(subcommand cmd.Command) cmd.Command {
	command := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:                "juju",
		UserAliasesFilename: osenv.JujuXDGDataHomePath("aliases"),
		FlagKnownAs:         "option",
	})
	command.Register(subcommand)
	return command
}

func (s *BaseBackupsSuite) checkArchive(c *gc.C) {
	c.Assert(s.filename, gc.Not(gc.Equals), "")
	archive, err := os.Open(s.filename)
	c.Assert(err, jc.ErrorIsNil)
	defer archive.Close()

	// Test file created successfully. Clean it up after the test is run.
	s.AddCleanup(func(c *gc.C) {
		err := os.Remove(s.filename)
		if !os.IsNotExist(err) {
			c.Fatalf("could not remove test file %v: %v", s.filename, err)
		}
	})

	data, err := ioutil.ReadAll(archive)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, s.data)
}

// TODO (hml) 2018-05-01
// Replace this fakeAPIClient with MockAPIClient for all tests.
type fakeAPIClient struct {
	metaresult *params.BackupsMetadataResult
	archive    io.ReadCloser
	err        error

	calls []string
	args  []string
	idArg string
	notes string
}

func (f *fakeAPIClient) Check(c *gc.C, id, notes string, calls ...string) {
	c.Check(f.calls, jc.DeepEquals, calls)
	c.Check(f.idArg, gc.Equals, id)
	c.Check(f.notes, gc.Equals, notes)
}

func (f *fakeAPIClient) CheckCalls(c *gc.C, calls ...string) {
	c.Check(f.calls, jc.DeepEquals, calls)
}

func (f *fakeAPIClient) CheckArgs(c *gc.C, args ...string) {
	c.Check(f.args, jc.DeepEquals, args)
}

func (c *fakeAPIClient) Create(notes string, keepCopy, noDownload bool) (*params.BackupsMetadataResult, error) {
	c.calls = append(c.calls, "Create")
	c.args = append(c.args, notes, fmt.Sprintf("%t", keepCopy), fmt.Sprintf("%t", noDownload))
	c.notes = notes
	if c.err != nil {
		return nil, c.err
	}
	createResult := c.metaresult

	return createResult, nil
}

func (c *fakeAPIClient) Info(id string) (*params.BackupsMetadataResult, error) {
	c.calls = append(c.calls, "Info")
	c.args = append(c.args, id)
	c.idArg = id
	if c.err != nil {
		return nil, c.err
	}
	return c.metaresult, nil
}

func (c *fakeAPIClient) List() (*params.BackupsListResult, error) {
	c.calls = append(c.calls, "List")
	if c.err != nil {
		return nil, c.err
	}
	var result params.BackupsListResult
	result.List = []params.BackupsMetadataResult{*c.metaresult}
	return &result, nil
}

func (c *fakeAPIClient) Download(id string) (io.ReadCloser, error) {
	c.calls = append(c.calls, "Download")
	c.args = append(c.args, id)
	if c.err != nil {
		return nil, c.err
	}
	return c.archive, nil
}

func (c *fakeAPIClient) Upload(ar io.ReadSeeker, meta params.BackupsMetadataResult) (string, error) {
	c.args = append(c.args, "ar", "meta")
	if c.err != nil {
		return "", c.err
	}
	return c.metaresult.ID, nil
}

func (c *fakeAPIClient) Remove(id ...string) ([]params.ErrorResult, error) {
	c.calls = append(c.calls, "Remove")
	c.args = append(c.args, "id")
	c.idArg = id[0]
	if c.err != nil {
		return []params.ErrorResult{
			{Error: &params.Error{Message: c.err.Error()}},
		}, nil
	}
	return nil, nil
}

func (c *fakeAPIClient) Close() error {
	return nil
}

func (c *fakeAPIClient) RestoreReader(io.ReadSeeker, *params.BackupsMetadataResult, apibackups.ClientConnection) error {
	return nil
}

func (c *fakeAPIClient) Restore(string, apibackups.ClientConnection) error {
	return nil
}
