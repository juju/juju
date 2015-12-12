// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
	"strings"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/cmd"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ShowSuite{})

type ShowSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *stubCharmStore
}

func (s *ShowSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &stubCharmStore{stub: s.stub}
}

func (s *ShowSuite) newAPIClient(c *cmd.ShowCommand) (cmd.CharmStore, error) {
	s.stub.AddCall("newAPIClient", c)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.client, nil
}

func (s *ShowSuite) TestInfo(c *gc.C) {
	var command cmd.ShowCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "show-charm-resources",
		Args:    "charm-id",
		Purpose: "display the resources for a charm in the charm store",
		Doc: `
This command will report the resources for a charm in the charm store.
`,
	})
}

func (s *ShowSuite) TestOkay(c *gc.C) {
	infos := cmd.NewInfos(c,
		"website:.tgz of your website",
		"music:mp3 of your backing vocals",
	)
	s.client.ReturnListResources = [][]resource.Info{infos}

	command := cmd.NewShowCommand(s.newAPIClient)
	code, stdout, stderr := runShow(c, command, "cs:a-charm")
	c.Check(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
RESOURCE FROM   REV COMMENT                    
website  upload -   .tgz of your website       
music    upload -   mp3 of your backing vocals 

`[1:])
	c.Check(stderr, gc.Equals, "")
	s.stub.CheckCallNames(c, "newAPIClient", "ListResources", "Close")
	s.stub.CheckCall(c, 0, "newAPIClient", command)
	s.stub.CheckCall(c, 1, "ListResources", []string{"cs:a-charm"})
}

func (s *ShowSuite) TestNoResources(c *gc.C) {
	s.client.ReturnListResources = [][]resource.Info{{}}

	command := cmd.NewShowCommand(s.newAPIClient)
	code, stdout, stderr := runShow(c, command, "cs:a-charm")
	c.Check(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
RESOURCE FROM REV COMMENT 

`[1:])
	c.Check(stderr, gc.Equals, "")
	s.stub.CheckCallNames(c, "newAPIClient", "ListResources", "Close")
}

func (s *ShowSuite) TestOutputFormats(c *gc.C) {
	fp1, err := charmresource.GenerateFingerprint([]byte("abc"))
	c.Assert(err, jc.ErrorIsNil)
	fp2, err := charmresource.GenerateFingerprint([]byte("xyz"))
	c.Assert(err, jc.ErrorIsNil)
	infos := []resource.Info{
		cmd.NewInfo(c, "website", ".tgz", ".tgz of your website", string(fp1.Bytes())),
		cmd.NewInfo(c, "music", ".mp3", "mp3 of your backing vocals", string(fp2.Bytes())),
	}
	s.client.ReturnListResources = [][]resource.Info{infos}

	formats := map[string]string{
		"tabular": `
RESOURCE FROM   REV COMMENT                    
website  upload -   .tgz of your website       
music    upload -   mp3 of your backing vocals 

`[1:],
		"yaml": `
- name: website
  type: file
  path: website.tgz
  comment: .tgz of your website
  fingerprint: cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7
  origin: upload
- name: music
  type: file
  path: music.mp3
  comment: mp3 of your backing vocals
  fingerprint: edcb0f4721e6578d900e4c24ad4b19e194ab6c87f8243bfc6b11754dd8b0bbde4f30b1d18197932b6376da004dcd97c4
  origin: upload
`[1:],
		"json": strings.Replace(""+
			"["+
			"  {"+
			`    "name":"website",`+
			`    "type":"file",`+
			`    "path":"website.tgz",`+
			`    "comment":".tgz of your website",`+
			`    "fingerprint":"cb00753f45a35e8bb5a03d699ac65007272c32ab0eded1631a8b605a43ff5bed8086072ba1e7cc2358baeca134c825a7",`+
			`    "origin":"upload"`+
			"  },{"+
			`    "name":"music",`+
			`    "type":"file",`+
			`    "path":"music.mp3",`+
			`    "comment":"mp3 of your backing vocals",`+
			`    "fingerprint":"edcb0f4721e6578d900e4c24ad4b19e194ab6c87f8243bfc6b11754dd8b0bbde4f30b1d18197932b6376da004dcd97c4",`+
			`    "origin":"upload"`+
			"  }"+
			"]\n",
			"  ", "", -1),
	}
	for format, expected := range formats {
		command := cmd.NewShowCommand(s.newAPIClient)
		args := []string{
			"--format", format,
			"cs:a-charm",
		}
		code, stdout, stderr := runShow(c, command, args...)
		c.Check(code, gc.Equals, 0)

		c.Check(stdout, gc.Equals, expected)
		c.Check(stderr, gc.Equals, "")
	}
}

func runShow(c *gc.C, command *cmd.ShowCommand, args ...string) (int, string, string) {
	ctx := coretesting.Context(c)
	code := jujucmd.Main(command, ctx, args)
	stdout := ctx.Stdout.(*bytes.Buffer).Bytes()
	stderr := ctx.Stderr.(*bytes.Buffer).Bytes()
	return code, string(stdout), string(stderr)
}
