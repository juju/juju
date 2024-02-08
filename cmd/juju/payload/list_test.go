// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/payload"
	corepayloads "github.com/juju/juju/core/payloads"
)

var _ = gc.Suite(&listSuite{})

type listSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *stubClient
}

func (s *listSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &stubClient{stub: s.stub}
}

func (s *listSuite) newAPIClient() (payload.ListAPI, error) {
	return s.client, nil
}

func (s *listSuite) TestInfo(c *gc.C) {
	var command payload.ListCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &cmd.Info{
		Name:    "payloads",
		Args:    "[pattern ...]",
		Purpose: "Display status information about known payloads.",
		Doc: `
This command will report on the runtime state of defined payloads.

When one or more pattern is given, Juju will limit the results to only
those payloads which match *any* of the provided patterns. Each pattern
will be checked against the following info in Juju:

- unit name
- machine id
- payload type
- payload class
- payload id
- payload tag
- payload status
`,
		Aliases:        []string{"list-payloads"},
		FlagKnownAs:    "option",
		ShowSuperFlags: []string{"show-log", "debug", "logging-config", "verbose", "quiet", "h", "help"},
	})
}

func (s *listSuite) TestList(c *gc.C) {
	p1 := payload.NewPayload("spam", "a-application", 1, 0)
	p1.Labels = []string{"a-tag"}
	p2 := payload.NewPayload("eggs", "another-application", 2, 1)
	s.client.payloads = append(s.client.payloads, p1, p2)

	command := payload.NewListCommandForTest(s.newAPIClient)
	code, stdout, stderr := runList(c, command)
	c.Assert(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
[Unit Payloads]
Unit                   Machine  Payload class  Status   Type    Id      Tags   
a-application/0        1        spam           running  docker  idspam  a-tag  
another-application/1  2        eggs           running  docker  ideggs         
`[1:])
	c.Check(stderr, gc.Equals, "")
}

func (s *listSuite) TestNoPayloads(c *gc.C) {
	command := payload.NewListCommandForTest(s.newAPIClient)
	code, stdout, stderr := runList(c, command)
	c.Assert(code, gc.Equals, 0)

	c.Check(stderr, gc.Equals, "No payloads to display.\n")
	c.Check(stdout, gc.Equals, "")
}

func (s *listSuite) TestPatternsOkay(c *gc.C) {
	p1 := payload.NewPayload("spam", "a-application", 1, 0)
	p1.Labels = []string{"a-tag"}
	p2 := payload.NewPayload("eggs", "another-application", 2, 1)
	p2.Labels = []string{"a-tag"}
	s.client.payloads = append(s.client.payloads, p1, p2)

	command := payload.NewListCommandForTest(s.newAPIClient)
	args := []string{
		"a-tag",
		"other",
		"some-application/1",
	}
	code, stdout, stderr := runList(c, command, args...)
	c.Assert(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
[Unit Payloads]
Unit                   Machine  Payload class  Status   Type    Id      Tags   
a-application/0        1        spam           running  docker  idspam  a-tag  
another-application/1  2        eggs           running  docker  ideggs  a-tag  
`[1:])
	c.Check(stderr, gc.Equals, "")
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "List",
		Args: []interface{}{
			[]string{
				"a-tag",
				"other",
				"some-application/1",
			},
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *listSuite) TestOutputFormats(c *gc.C) {
	p1 := payload.NewPayload("spam", "a-application", 1, 0)
	p1.Labels = []string{"a-tag"}
	p2 := payload.NewPayload("eggs", "another-application", 2, 1)
	s.client.payloads = append(s.client.payloads,
		p1,
		p2,
	)

	formats := map[string]string{
		"tabular": `
[Unit Payloads]
Unit                   Machine  Payload class  Status   Type    Id      Tags   
a-application/0        1        spam           running  docker  idspam  a-tag  
another-application/1  2        eggs           running  docker  ideggs         
`[1:],
		"yaml": `
- unit: a-application/0
  machine: "1"
  id: idspam
  type: docker
  payload-class: spam
  tags:
  - a-tag
  status: running
- unit: another-application/1
  machine: "2"
  id: ideggs
  type: docker
  payload-class: eggs
  status: running
`[1:],
		"json": strings.Replace(""+
			"["+
			" {"+
			`  "unit":"a-application/0",`+
			`  "machine":"1",`+
			`  "id":"idspam",`+
			`  "type":"docker",`+
			`  "payload-class":"spam",`+
			`  "tags":["a-tag"],`+
			`  "status":"running"`+
			" },{"+
			`  "unit":"another-application/1",`+
			`  "machine":"2",`+
			`  "id":"ideggs",`+
			`  "type":"docker",`+
			`  "payload-class":"eggs",`+
			`  "status":"running"`+
			" }"+
			"]\n",
			" ", "", -1),
	}
	for format, expected := range formats {
		command := payload.NewListCommandForTest(s.newAPIClient)
		args := []string{
			"--format", format,
		}
		code, stdout, stderr := runList(c, command, args...)
		c.Assert(code, gc.Equals, 0)

		c.Check(stdout, gc.Equals, expected)
		c.Check(stderr, gc.Equals, "")
	}
}

func runList(c *gc.C, command *payload.ListCommand, args ...string) (int, string, string) {
	ctx := cmdtesting.Context(c)
	code := cmd.Main(command, ctx, args)
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr := ctx.Stderr.(*bytes.Buffer).Bytes()
	return code, string(stdout), string(stderr)
}

type stubClient struct {
	stub     *testing.Stub
	payloads []corepayloads.FullPayloadInfo
}

func (s *stubClient) ListFull(patterns ...string) ([]corepayloads.FullPayloadInfo, error) {
	s.stub.AddCall("List", patterns)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.payloads, nil
}

func (s *stubClient) Close() error {
	s.stub.AddCall("Close")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
