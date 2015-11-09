// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"bytes"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/status"
	coretesting "github.com/juju/juju/testing"
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

func (s *listSuite) newAPIClient(c *status.ListCommand) (status.ListAPI, error) {
	s.stub.AddCall("newAPIClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *listSuite) TestInfo(c *gc.C) {
	var command status.ListCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &cmd.Info{
		Name:    "list-payloads",
		Args:    "[pattern ...]",
		Purpose: "display status information about known payloads",
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
	})
}

func (s *listSuite) TestOkay(c *gc.C) {
	p1 := status.NewPayload("spam", "a-service", 1, 0)
	p1.Labels = []string{"a-tag"}
	p2 := status.NewPayload("eggs", "another-service", 2, 1)
	s.client.payloads = append(s.client.payloads, p1, p2)

	command := status.NewListCommand(s.newAPIClient)
	code, stdout, stderr := runList(c, command)
	c.Assert(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
[Unit Payloads]
UNIT              MACHINE PAYLOAD-CLASS STATUS  TYPE   ID     TAGS  
a-service/0       1       spam          running docker idspam a-tag 
another-service/1 2       eggs          running docker ideggs       

`[1:])
	c.Check(stderr, gc.Equals, "")
}

func (s *listSuite) TestNoPayloads(c *gc.C) {
	command := status.NewListCommand(s.newAPIClient)
	code, stdout, stderr := runList(c, command)
	c.Assert(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
[Unit Payloads]
UNIT MACHINE PAYLOAD-CLASS STATUS TYPE ID TAGS 

`[1:])
	c.Check(stderr, gc.Equals, "")
}

func (s *listSuite) TestPatternsOkay(c *gc.C) {
	p1 := status.NewPayload("spam", "a-service", 1, 0)
	p1.Labels = []string{"a-tag"}
	p2 := status.NewPayload("eggs", "another-service", 2, 1)
	p2.Labels = []string{"a-tag"}
	s.client.payloads = append(s.client.payloads, p1, p2)

	command := status.NewListCommand(s.newAPIClient)
	args := []string{
		"a-tag",
		"other",
		"some-service/1",
	}
	code, stdout, stderr := runList(c, command, args...)
	c.Assert(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
[Unit Payloads]
UNIT              MACHINE PAYLOAD-CLASS STATUS  TYPE   ID     TAGS  
a-service/0       1       spam          running docker idspam a-tag 
another-service/1 2       eggs          running docker ideggs a-tag 

`[1:])
	c.Check(stderr, gc.Equals, "")
	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "newAPIClient",
		Args: []interface{}{
			command,
		},
	}, {
		FuncName: "List",
		Args: []interface{}{
			[]string{
				"a-tag",
				"other",
				"some-service/1",
			},
		},
	}, {
		FuncName: "Close",
	}})
}

func (s *listSuite) TestOutputFormats(c *gc.C) {
	p1 := status.NewPayload("spam", "a-service", 1, 0)
	p1.Labels = []string{"a-tag"}
	p2 := status.NewPayload("eggs", "another-service", 2, 1)
	s.client.payloads = append(s.client.payloads,
		p1,
		p2,
	)

	formats := map[string]string{
		"tabular": `
[Unit Payloads]
UNIT              MACHINE PAYLOAD-CLASS STATUS  TYPE   ID     TAGS  
a-service/0       1       spam          running docker idspam a-tag 
another-service/1 2       eggs          running docker ideggs       

`[1:],
		"yaml": `
- unit: a-service/0
  machine: "1"
  id: idspam
  type: docker
  payload-class: spam
  tags:
  - a-tag
  status: running
- unit: another-service/1
  machine: "2"
  id: ideggs
  type: docker
  payload-class: eggs
  status: running
`[1:],
		"json": strings.Replace(""+
			"["+
			" {"+
			`  "unit":"a-service/0",`+
			`  "machine":"1",`+
			`  "id":"idspam",`+
			`  "type":"docker",`+
			`  "payload-class":"spam",`+
			`  "tags":["a-tag"],`+
			`  "status":"running"`+
			" },{"+
			`  "unit":"another-service/1",`+
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
		command := status.NewListCommand(s.newAPIClient)
		args := []string{
			"--format", format,
		}
		code, stdout, stderr := runList(c, command, args...)
		c.Assert(code, gc.Equals, 0)

		c.Check(stdout, gc.Equals, expected)
		c.Check(stderr, gc.Equals, "")
	}
}

func runList(c *gc.C, command *status.ListCommand, args ...string) (int, string, string) {
	ctx := coretesting.Context(c)
	code := cmd.Main(command, ctx, args)
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr := ctx.Stderr.(*bytes.Buffer).Bytes()
	return code, string(stdout), string(stderr)
}

type stubClient struct {
	stub     *testing.Stub
	payloads []payload.FullPayloadInfo
}

func (s *stubClient) ListFull(patterns ...string) ([]payload.FullPayloadInfo, error) {
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
