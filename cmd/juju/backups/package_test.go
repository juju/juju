// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"os"

	"github.com/juju/tc"

	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/api/jujuclient/jujuclienttesting"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/juju/backups"
	"github.com/juju/juju/core/model"
	jujutesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/osenv"
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

checksum:              YuLeCCL75ZT/frYSABmKhamUh58= 
checksum format:        
size (B):              0 
stored:                0001-01-01 00:00:00 +0000 UTC 
started:               0001-01-01 00:00:00 +0000 UTC 
finished:              0001-01-01 00:00:00 +0000 UTC 

notes:                  

`[1:]

type BaseBackupsSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite

	metaresult *params.BackupsMetadataResult
	data       string

	filename string

	store *jujuclient.MemStore
}

func (s *BaseBackupsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	c.Chdir(c.MkDir())

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

func (s *BaseBackupsSuite) patchGetAPI(client backups.APIClient) {
	s.PatchValue(backups.NewGetAPI,
		func(ctx context.Context, c *backups.CommandBase) (backups.APIClient, error) {
			return client, nil
		},
	)
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

	data, err := io.ReadAll(archive)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, s.data)
}

// TODO (hml) 2018-05-01
// Replace this fakeAPIClient with MockAPIClient for all tests.
type fakeAPIClient struct {
	metaresult *params.BackupsMetadataResult
	data       string
	err        error

	// createHook, when set, is called for the Create invocation and
	// its reader replaces the default archive stream. It lets tests
	// script transfer behaviour, e.g. a reader that fails part way.
	createHook func() (io.ReadCloser, error)

	calls []string
	args  []string
	notes string
}

func (f *fakeAPIClient) Check(c *tc.C, notes string, calls ...string) {
	c.Check(f.calls, tc.DeepEquals, calls)
	c.Check(f.notes, tc.Equals, notes)
}

func (f *fakeAPIClient) CheckCalls(c *tc.C, calls ...string) {
	c.Check(f.calls, tc.DeepEquals, calls)
}

func (f *fakeAPIClient) CheckArgs(c *tc.C, args ...string) {
	c.Check(f.args, tc.DeepEquals, args)
}

func (c *fakeAPIClient) Create(ctx context.Context, notes string) (params.BackupsMetadataResult, io.ReadCloser, error) {
	c.calls = append(c.calls, "Create")
	c.args = append(c.args, notes)
	c.notes = notes
	if c.err != nil {
		return params.BackupsMetadataResult{}, nil, c.err
	}
	if c.data != "" {
		// The archive checksum matches the streamed data unless the
		// test set one explicitly.
		if c.metaresult.Checksum == "" {
			sum := sha1.Sum([]byte(c.data))
			c.metaresult.Checksum = base64.StdEncoding.EncodeToString(sum[:])
		}
	}
	if c.createHook != nil {
		rdr, err := c.createHook()
		if err != nil {
			return params.BackupsMetadataResult{}, nil, err
		}
		return *c.metaresult, rdr, nil
	}
	return *c.metaresult, io.NopCloser(bytes.NewReader([]byte(c.data))), nil
}

func (c *fakeAPIClient) Close() error {
	return nil
}
