// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource_test

import (
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/resource"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/testhelpers"
)

var _ = tc.Suite(&CharmResourcesSuite{})

type CharmResourcesSuite struct {
	testhelpers.IsolationSuite

	stub   *testhelpers.Stub
	client *stubCharmStore
}

func (s *CharmResourcesSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testhelpers.Stub{}
	s.client = &stubCharmStore{stub: s.stub}
}

func (s *CharmResourcesSuite) TestInfo(c *tc.C) {
	var command resource.CharmResourcesCommand
	info := command.Info()

	// Verify that Info is wired up. Without verifying exact text.
	c.Check(info.Name, tc.Equals, "charm-resources")
	c.Check(info.Aliases, tc.Not(tc.Equals), "")
	c.Check(info.Purpose, tc.Not(tc.Equals), "")
	c.Check(info.Doc, tc.Not(tc.Equals), "")
	c.Check(info.Examples, tc.Not(tc.Equals), "")
	c.Check(info.FlagKnownAs, tc.Not(tc.Equals), "")
	c.Check(len(info.ShowSuperFlags), tc.GreaterThan, 2)
}

func (s *CharmResourcesSuite) TestOkay(c *tc.C) {
	resources := newCharmResources(c,
		"website:.tgz of your website",
		"music:mp3 of your backing vocals",
	)
	resources[0].Revision = 2
	s.client.ReturnListResources = [][]charmresource.Resource{resources}

	command := resource.NewCharmResourcesCommandForTest(s.client)
	code, stdout, stderr := runCmd(c, command, "a-charm")
	c.Check(code, tc.Equals, 0)

	c.Check(stdout, tc.Equals, `
Resource  Revision
music     1
website   2
`[1:])
	c.Check(stderr, tc.Equals, "")
	s.stub.CheckCallNames(c,
		"ListResources",
	)
	s.stub.CheckCall(c, 0, "ListResources", []resource.CharmID{
		{
			URL:     charm.MustParseURL("a-charm"),
			Channel: corecharm.MustParseChannel("stable"),
		},
	})
}

func (s *CharmResourcesSuite) TestNoResources(c *tc.C) {
	s.client.ReturnListResources = [][]charmresource.Resource{{}}

	command := resource.NewCharmResourcesCommandForTest(s.client)
	code, stdout, stderr := runCmd(c, command, "a-charm")
	c.Check(code, tc.Equals, 0)

	c.Check(stderr, tc.Equals, "No resources to display.\n")
	c.Check(stdout, tc.Equals, "")
	s.stub.CheckCallNames(c, "ListResources")
}

func (s *CharmResourcesSuite) TestOutputFormats(c *tc.C) {
	fp1, err := charmresource.GenerateFingerprint(strings.NewReader("abc"))
	c.Assert(err, tc.ErrorIsNil)
	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("xyz"))
	c.Assert(err, tc.ErrorIsNil)
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
		command := resource.NewCharmResourcesCommandForTest(s.client)
		args := []string{
			"--format", format,
			"ch:a-charm",
		}
		code, stdout, stderr := runCmd(c, command, args...)
		c.Check(code, tc.Equals, 0)

		c.Check(stdout, tc.Equals, expected)
		c.Check(stderr, tc.Equals, "")
	}
}

func (s *CharmResourcesSuite) TestChannelFlag(c *tc.C) {
	fp1, err := charmresource.GenerateFingerprint(strings.NewReader("abc"))
	c.Assert(err, tc.ErrorIsNil)
	fp2, err := charmresource.GenerateFingerprint(strings.NewReader("xyz"))
	c.Assert(err, tc.ErrorIsNil)
	resources := []charmresource.Resource{
		charmRes(c, "website", ".tgz", ".tgz of your website", string(fp1.Bytes())),
		charmRes(c, "music", ".mp3", "mp3 of your backing vocals", string(fp2.Bytes())),
	}
	s.client.ReturnListResources = [][]charmresource.Resource{resources}
	command := resource.NewCharmResourcesCommandForTest(s.client)

	code, _, stderr := runCmd(c, command,
		"--channel", "development",
		"ch:a-charm",
	)

	c.Check(code, tc.Equals, 0)
	c.Check(stderr, tc.Equals, "")
	c.Check(resource.CharmResourcesCommandChannel(command), tc.Equals, "development")
}
