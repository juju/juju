// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/cmd"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

// MetaResultString is the expected output of running dumpMetadata() on
// s.metaresult.
var MetaResultString = `

backup format version: 0 
juju version:          0.0.0 
base:                   

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
	tc.TestingT(t)
}

type BaseBackupsSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite

	metaresult *params.BackupsMetadataResult
	data       string

	filename string

	store *jujuclient.MemStore
}

func (s *BaseBackupsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.metaresult = &params.BackupsMetadataResult{
		ID:       "backup-id",
		Filename: "backup-filename",
	}
	s.data = "<compressed archive data>"

	s.store = jujuclienttesting.MinimalStore()
	models := s.store.Models["arthur"]
	models.Models["admin/controller"] = jujuclient.ModelDetails{
		ModelUUID: uuid.MustNewUUID().String(),
		ModelType: model.IAAS,
	}
	s.store.Models["arthur"] = models
}

func (s *BaseBackupsSuite) TearDownTest(c *tc.C) {
	if s.filename != "" {
		err := os.Remove(s.filename)
		if !os.IsNotExist(err) {
			c.Check(err, tc.ErrorIsNil)
		}
	}

	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *BaseBackupsSuite) patchAPIClient(client backups.APIClient) {
	s.PatchValue(backups.NewAPIClient,
		func(ctx context.Context, c *backups.CommandBase) (backups.APIClient, error) {
			return client, nil
		},
	)
}

func (s *BaseBackupsSuite) patchGetAPI(client backups.APIClient) {
	s.PatchValue(backups.NewGetAPI,
		func(ctx context.Context, c *backups.CommandBase) (backups.APIClient, error) {
			return client, nil
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
	client.archive = io.NopCloser(bytes.NewBufferString(s.data))
	return client
}

func (s *BaseBackupsSuite) createCommandForGlobalOptionTesting(subcommand cmd.Command) cmd.Command {
	command := jujucmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:                "juju",
		UserAliasesFilename: osenv.JujuXDGDataHomePath("aliases"),
		FlagKnownAs:         "option",
		Log:                 jujucmd.DefaultLog,
	})
	command.Register(subcommand)
	return command
}

func (s *BaseBackupsSuite) checkArchive(c *tc.C) {
	c.Assert(s.filename, tc.Not(tc.Equals), "")
	archive, err := os.Open(s.filename)
	c.Assert(err, tc.ErrorIsNil)
	defer archive.Close()

	// Test file created successfully. Clean it up after the test is run.
	s.AddCleanup(func(c *tc.C) {
		err := os.Remove(s.filename)
		if !os.IsNotExist(err) {
			c.Fatalf("could not remove test file %v: %v", s.filename, err)
		}
	})

	data, err := io.ReadAll(archive)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, s.data)
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

func (f *fakeAPIClient) Check(c *tc.C, id, notes string, calls ...string) {
	c.Check(f.calls, tc.DeepEquals, calls)
	c.Check(f.idArg, tc.Equals, id)
	c.Check(f.notes, tc.Equals, notes)
}

func (f *fakeAPIClient) CheckCalls(c *tc.C, calls ...string) {
	c.Check(f.calls, tc.DeepEquals, calls)
}

func (f *fakeAPIClient) CheckArgs(c *tc.C, args ...string) {
	c.Check(f.args, tc.DeepEquals, args)
}

func (c *fakeAPIClient) Create(ctx context.Context, notes string, noDownload bool) (*params.BackupsMetadataResult, error) {
	c.calls = append(c.calls, "Create")
	c.args = append(c.args, notes, fmt.Sprintf("%t", noDownload))
	c.notes = notes
	if c.err != nil {
		return nil, c.err
	}
	createResult := c.metaresult

	return createResult, nil
}

func (c *fakeAPIClient) Download(_ context.Context, id string) (io.ReadCloser, error) {
	c.calls = append(c.calls, "Download")
	c.args = append(c.args, id)
	if c.err != nil {
		return nil, c.err
	}
	return c.archive, nil
}

func (c *fakeAPIClient) Close() error {
	return nil
}
