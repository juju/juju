// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

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
environment ID:  ""
machine ID:      ""
created on host: ""
juju version:    0.0.0
`[1:]

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseBackupsSuite struct {
	jujutesting.FakeJujuHomeSuite
	command    *backups.Command
	metaresult *params.BackupsMetadataResult
}

func (s *BaseBackupsSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.command = backups.NewCommand().(*backups.Command)
	s.metaresult = &params.BackupsMetadataResult{
		ID: "spam",
	}
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

func (s *BaseBackupsSuite) diffStrings(c *gc.C, value, expected string) {
	// If only Go had a diff library.
	vlines := strings.Split(value, "\n")
	elines := strings.Split(expected, "\n")
	vsize := len(vlines)
	esize := len(elines)

	if vsize < 2 || esize < 2 {
		return
	}

	smaller := elines
	if vsize < esize {
		smaller = vlines
	}

	for i, _ := range smaller {
		vline := vlines[i]
		eline := elines[i]
		if vline != eline {
			c.Log("first mismatched line:")
			c.Log("expected: " + eline)
			c.Log("got:      " + vline)
			break
		}
	}

}

func (s *BaseBackupsSuite) checkString(c *gc.C, value, expected string) {
	if !c.Check(value, gc.Equals, expected) {
		s.diffStrings(c, value, expected)
	}
}

func (s *BaseBackupsSuite) checkStd(c *gc.C, ctx *cmd.Context, out, err string) {
	c.Check(ctx.Stdin.(*bytes.Buffer).String(), gc.Equals, "")
	s.checkString(c, ctx.Stdout.(*bytes.Buffer).String(), out)
	s.checkString(c, ctx.Stderr.(*bytes.Buffer).String(), err)
}

type fakeAPIClient struct {
	metaresult *params.BackupsMetadataResult
	archive    io.ReadCloser
	err        error

	args  []string
	idArg string
	notes string
}

func (c *fakeAPIClient) Create(notes string) (*params.BackupsMetadataResult, error) {
	c.args = append(c.args, "notes")
	c.notes = notes
	if c.err != nil {
		return nil, c.err
	}
	return c.metaresult, nil
}

func (c *fakeAPIClient) Info(id string) (*params.BackupsMetadataResult, error) {
	c.args = append(c.args, "id")
	c.idArg = id
	if c.err != nil {
		return nil, c.err
	}
	return c.metaresult, nil
}

func (c *fakeAPIClient) List() (*params.BackupsListResult, error) {
	if c.err != nil {
		return nil, c.err
	}
	var result params.BackupsListResult
	result.List = []params.BackupsMetadataResult{*c.metaresult}
	return &result, nil
}

func (c *fakeAPIClient) Download(id string) (io.ReadCloser, error) {
	c.args = append(c.args, "id")
	c.idArg = id
	if c.err != nil {
		return nil, c.err
	}
	return c.archive, nil
}

func (c *fakeAPIClient) Remove(id string) error {
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

func (c *fakeAPIClient) PrepareRestore() error {
	return nil
}

func (c *fakeAPIClient) FinishRestore() error {
	return nil
}

func (c *fakeAPIClient) Restore(string, string) error {
	return nil
}

func (c *fakeAPIClient) PublicAddress(string) (string, error) {
	return "", nil
}
