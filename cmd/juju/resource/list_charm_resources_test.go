// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	jujucmd "github.com/juju/cmd"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmstore"
	resourcecmd "github.com/juju/juju/cmd/juju/resource"
)

var _ = gc.Suite(&ListCharmSuite{})

type ListCharmSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *stubCharmStore
}

func (s *ListCharmSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &stubCharmStore{stub: s.stub}
}

func (s *ListCharmSuite) TestInfo(c *gc.C) {
	var command resourcecmd.ListCharmResourcesCommand
	info := command.Info()

	c.Check(info, jc.DeepEquals, &jujucmd.Info{
		Name:    "resources",
		Args:    "<charm>",
		Purpose: "DEPRECATED: Display the resources for a charm in the charm store.",
		Doc: `This command is DEPRECATED since Juju 2.3.x, please use 'juju charm-resources' instead.

This command will report the resources for a charm in the charm store.

<charm> can be a charm URL, or an unambiguously condensed form of it,
just like the deploy command. So the following forms will be accepted:

For cs:trusty/mysql
  mysql
  trusty/mysql

For cs:~user/trusty/mysql
  cs:~user/mysql

Where the series is not supplied, the series from your local host is used.
Thus the above examples imply that the local series is trusty.
`,
		Aliases:        []string{"list-resources"},
		FlagKnownAs:    "option",
		ShowSuperFlags: []string{"show-log", "debug", "logging-config", "verbose", "quiet", "h", "help"},
	})
}

func (s *ListCharmSuite) TestOkay(c *gc.C) {
	resources := newCharmResources(c,
		"website:.tgz of your website",
		"music:mp3 of your backing vocals",
	)
	resources[0].Revision = 2
	s.client.ReturnListResources = [][]charmresource.Resource{resources}

	command := resourcecmd.NewListCharmResourcesCommandForTest(s.client)
	code, stdout, stderr := runCmd(c, command, "cs:a-charm")
	c.Check(code, gc.Equals, 0)

	c.Check(stdout, gc.Equals, `
Resource  Revision
music     1
website   2

`[1:])
	c.Check(stderr, gc.Equals, "")
	s.stub.CheckCallNames(c,
		"ListResources",
	)
	s.stub.CheckCall(c, 0, "ListResources", []charmstore.CharmID{
		{
			URL:     charm.MustParseURL("cs:a-charm"),
			Channel: "stable",
		},
	})
}

func (s *ListCharmSuite) TestNoResources(c *gc.C) {
	s.client.ReturnListResources = [][]charmresource.Resource{{}}

	command := resourcecmd.NewListCharmResourcesCommandForTest(s.client)
	code, stdout, stderr := runCmd(c, command, "cs:a-charm")
	c.Check(code, gc.Equals, 0)

	c.Check(stderr, gc.Equals, "No resources to display.\n")
	c.Check(stdout, gc.Equals, "")
	s.stub.CheckCallNames(c, "ListResources")
}

func (s *ListCharmSuite) TestOutputFormats(c *gc.C) {
	fp1, err := charmresource.GenerateFingerprint(strings.NewReader("abc"))
	c.Assert(err, jc.ErrorIsNil)
	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("xyz"))
	c.Assert(err, jc.ErrorIsNil)
	resources := []charmresource.Resource{
		charmRes(c, "website", ".tgz", ".tgz of your website", string(fp1.Bytes())),
		charmRes(c, "music", ".mp3", "mp3 of your backing vocals", string(fp2.Bytes())),
	}
	s.client.ReturnListResources = [][]charmresource.Resource{resources}

	formats := map[string]string{
		"tabular": `
Resource  Revision
music     1
website   1

`[1:],
		"yaml": `
- name: music
  type: file
  path: music.mp3
  description: mp3 of your backing vocals
  revision: 1
  fingerprint: b0ea2a0f90267a8bd32848c65d7a61569a136f4e421b56127b6374b10a576d29e09294e620b4dcdee40f602115104bd5
  size: 48
  origin: store
- name: website
  type: file
  path: website.tgz
  description: .tgz of your website
  revision: 1
  fingerprint: 73100f01cf258766906c34a30f9a486f07259c627ea0696d97c4582560447f59a6df4a7cf960708271a30324b1481ef4
  size: 48
  origin: store
`[1:],
		"json": strings.Replace(""+
			"["+
			"  {"+
			`    "name":"music",`+
			`    "type":"file",`+
			`    "path":"music.mp3",`+
			`    "description":"mp3 of your backing vocals",`+
			`    "revision":1,`+
			`    "fingerprint":"b0ea2a0f90267a8bd32848c65d7a61569a136f4e421b56127b6374b10a576d29e09294e620b4dcdee40f602115104bd5",`+
			`    "size":48,`+
			`    "origin":"store"`+
			"  },{"+
			`    "name":"website",`+
			`    "type":"file",`+
			`    "path":"website.tgz",`+
			`    "description":".tgz of your website",`+
			`    "revision":1,`+
			`    "fingerprint":"73100f01cf258766906c34a30f9a486f07259c627ea0696d97c4582560447f59a6df4a7cf960708271a30324b1481ef4",`+
			`    "size":48,`+
			`    "origin":"store"`+
			"  }"+
			"]\n",
			"  ", "", -1),
	}
	for format, expected := range formats {
		c.Logf("checking format %q", format)
		command := resourcecmd.NewListCharmResourcesCommandForTest(s.client)
		args := []string{
			"--format", format,
			"cs:a-charm",
		}
		code, stdout, stderr := runCmd(c, command, args...)
		c.Check(code, gc.Equals, 0)

		c.Check(stdout, gc.Equals, expected)
		c.Check(stderr, gc.Equals, "")
	}
}

func (s *ListCharmSuite) TestChannelFlag(c *gc.C) {
	fp1, err := charmresource.GenerateFingerprint(strings.NewReader("abc"))
	c.Assert(err, jc.ErrorIsNil)
	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("xyz"))
	c.Assert(err, jc.ErrorIsNil)
	resources := []charmresource.Resource{
		charmRes(c, "website", ".tgz", ".tgz of your website", string(fp1.Bytes())),
		charmRes(c, "music", ".mp3", "mp3 of your backing vocals", string(fp2.Bytes())),
	}
	s.client.ReturnListResources = [][]charmresource.Resource{resources}
	command := resourcecmd.NewListCharmResourcesCommandForTest(s.client)

	code, _, stderr := runCmd(c, command,
		"--channel", "development",
		"cs:a-charm",
	)

	c.Check(code, gc.Equals, 0)
	c.Check(stderr, gc.Equals, "")
	c.Check(resourcecmd.ListCharmResourcesCommandChannel(command), gc.Equals, "development")
}
