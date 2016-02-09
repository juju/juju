// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apibackups "github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/backups"
	jujutesting "github.com/juju/juju/testing"
)

// MetaResultString is the expected output of running dumpMetadata() on
// s.metaresult.
var MetaResultString = `
backup ID:       "spam"
checksum:        ""
checksum format: ""
size (B):        0
stored:          0001-01-01 00:00:00 +0000 UTC
started:         0001-01-01 00:00:00 +0000 UTC
finished:        0001-01-01 00:00:00 +0000 UTC
notes:           ""
model ID:        ""
machine ID:      ""
created on host: ""
juju version:    0.0.0
`[1:]

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseBackupsSuite struct {
	jujutesting.FakeJujuXDGDataHomeSuite

	command    cmd.Command
	metaresult *params.BackupsMetadataResult
	data       string

	filename string
}

func (s *BaseBackupsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	s.command = backups.NewSuperCommand()
	s.metaresult = &params.BackupsMetadataResult{
		ID: "spam",
	}
	s.data = "<compressed archive data>"
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

func (s *BaseBackupsSuite) checkHelp(c *gc.C, subcmd cmd.Command) {
	ctx, err := jujutesting.RunCommand(c, s.command, subcmd.Info().Name, "--help")
	c.Assert(err, gc.IsNil)

	var expected string
	if subcmd.Info().Args != "" {
		expected = "(?sm).*^usage: juju backups " +
			regexp.QuoteMeta(subcmd.Info().Name) +
			` \[options\] ` + regexp.QuoteMeta(subcmd.Info().Args) + ".+"
	} else {
		expected = "(?sm).*^usage: juju backups " +
			regexp.QuoteMeta(subcmd.Info().Name) +
			` \[options\].+`
	}
	c.Check(jujutesting.Stdout(ctx), gc.Matches, expected)

	expected = "(?sm).*^purpose: " + regexp.QuoteMeta(subcmd.Info().Purpose) + "$.*"
	c.Check(jujutesting.Stdout(ctx), gc.Matches, expected)

	expected = "(?sm).*^" + regexp.QuoteMeta(subcmd.Info().Doc) + "$.*"
	c.Check(jujutesting.Stdout(ctx), gc.Matches, expected)
}

func (s *BaseBackupsSuite) patchAPIClient(client backups.APIClient) {
	s.PatchValue(backups.NewAPIClient,
		func(c *backups.CommandBase) (backups.APIClient, error) {
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
	client.archive = ioutil.NopCloser(bytes.NewBufferString(s.data))
	return client
}

func (s *BaseBackupsSuite) checkArchive(c *gc.C) {
	c.Assert(s.filename, gc.Not(gc.Equals), "")
	archive, err := os.Open(s.filename)
	c.Assert(err, jc.ErrorIsNil)
	defer archive.Close()

	data, err := ioutil.ReadAll(archive)
	c.Check(string(data), gc.Equals, s.data)
}

func (s *BaseBackupsSuite) checkStd(c *gc.C, ctx *cmd.Context, out, err string) {
	c.Check(ctx.Stdin.(*bytes.Buffer).Len(), gc.Equals, 0)
	jujutesting.CheckString(c, ctx.Stdout.(*bytes.Buffer).String(), out)
	jujutesting.CheckString(c, ctx.Stderr.(*bytes.Buffer).String(), err)
}

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

func (c *fakeAPIClient) Create(notes string) (*params.BackupsMetadataResult, error) {
	c.calls = append(c.calls, "Create")
	c.args = append(c.args, "notes")
	c.notes = notes
	if c.err != nil {
		return nil, c.err
	}
	return c.metaresult, nil
}

func (c *fakeAPIClient) Info(id string) (*params.BackupsMetadataResult, error) {
	c.calls = append(c.calls, "Info")
	c.args = append(c.args, "id")
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
	c.args = append(c.args, "id")
	c.idArg = id
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

func (c *fakeAPIClient) Remove(id string) error {
	c.calls = append(c.calls, "Remove")
	c.args = append(c.args, "id")
	c.idArg = id
	if c.err != nil {
		return c.err
	}
	return nil
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
