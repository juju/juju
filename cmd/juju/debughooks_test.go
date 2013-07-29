// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	"regexp"
)

var _ = Suite(&DebugHooksSuite{})

type DebugHooksSuite struct {
	SSHCommonSuite
}

const debugHooksArgs = `-l ubuntu -t ` + commonArgs

var debugHooksTests = []struct {
	args   []string
	code   int
	result string
	stderr string
}{
	{
		[]string{"mysql/0"},
		0,
		regexp.QuoteMeta(debugHooksArgs + "dummyenv-0.dns -- sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKAojIExvY2sgdGhlIGp1anUtPHVuaXQ+LWRlYnVnIGxvY2tmaWxlLgpmbG9jayAtbiA4IHx8IChlY2hvICJGYWlsZWQgdG8gYWNxdWlyZSAvdG1wL2p1anUtdW5pdC1teXNxbC0wLWRlYnVnLWhvb2tzOiB1bml0IGlzIGFscmVhZHkgYmVpbmcgZGVidWdnZWQiIDI+JjE7IGV4aXQgMSkKKAojIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KZXhlYyA4PiYtCgojIFdyaXRlIG91dCB0aGUgZGVidWctaG9va3MgYXJncy4KZWNobyAiIiA+IC90bXAvanVqdS11bml0LW15c3FsLTAtZGVidWctaG9va3MKCiMgTG9jayB0aGUganVqdS08dW5pdD4tZGVidWctZXhpdCBsb2NrZmlsZS4KZmxvY2sgLW4gOSB8fCBleGl0IDEKCiMgV2FpdCBmb3IgdG11eCB0byBiZSBpbnN0YWxsZWQuCndoaWxlIFsgISAtZiAvdXNyL2Jpbi90bXV4IF07IGRvCiAgICBzbGVlcCAxCmRvbmUKCmlmIFsgISAtZiB+Ly50bXV4LmNvbmYgXTsgdGhlbgogICAgICAgIGlmIFsgLWYgL3Vzci9zaGFyZS9ieW9idS9wcm9maWxlcy90bXV4IF07IHRoZW4KICAgICAgICAgICAgICAgICMgVXNlIGJ5b2J1L3RtdXggcHJvZmlsZSBmb3IgZmFtaWxpYXIga2V5YmluZGluZ3MgYW5kIGJyYW5kaW5nCiAgICAgICAgICAgICAgICBlY2hvICJzb3VyY2UtZmlsZSAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXgiID4gfi8udG11eC5jb25mCiAgICAgICAgZWxzZQogICAgICAgICAgICAgICAgIyBPdGhlcndpc2UsIHVzZSB0aGUgbGVnYWN5IGp1anUvdG11eCBjb25maWd1cmF0aW9uCiAgICAgICAgICAgICAgICBjYXQgPiB+Ly50bXV4LmNvbmYgPDxFTkQKCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgooCiAgICAjIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KICAgIGV4ZWMgOT4mLQogICAgIyBUaGUgYmVhdXR5IGJlbG93IGlzIGEgd29ya2Fyb3VuZCBmb3IgYSBidWcgaW4gdG11eCAoMS41IGluIE9uZWlyaWMpIG9yCiAgICAjIGVwb2xsIHRoYXQgZG9lc24ndCBzdXBwb3J0IC9kZXYvbnVsbCBvciB3aGF0ZXZlci4gIFdpdGhvdXQgaXQgdGhlCiAgICAjIGNvbW1hbmQgaGFuZ3MuCiAgICB0bXV4IG5ldy1zZXNzaW9uIC1kIC1zIG15c3FsLzAgMj4mMSB8IGNhdCA+IC9kZXYvbnVsbCB8fCB0cnVlCiAgICBleGVjIHRtdXggYXR0YWNoIC10IG15c3FsLzAKKQopIDk+L3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcy1leGl0CikgOD4vdG1wL2p1anUtdW5pdC1teXNxbC0wLWRlYnVnLWhvb2tzCmV4aXQgJD8K | base64 -d > $F; . $F'\n"),
		"",
	},
	{
		[]string{"mongodb/1"},
		0,
		regexp.QuoteMeta(debugHooksArgs + "dummyenv-2.dns -- sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKAojIExvY2sgdGhlIGp1anUtPHVuaXQ+LWRlYnVnIGxvY2tmaWxlLgpmbG9jayAtbiA4IHx8IChlY2hvICJGYWlsZWQgdG8gYWNxdWlyZSAvdG1wL2p1anUtdW5pdC1tb25nb2RiLTEtZGVidWctaG9va3M6IHVuaXQgaXMgYWxyZWFkeSBiZWluZyBkZWJ1Z2dlZCIgMj4mMTsgZXhpdCAxKQooCiMgQ2xvc2UgdGhlIGluaGVyaXRlZCBsb2NrIEZELCBvciB0bXV4IHdpbGwga2VlcCBpdCBvcGVuLgpleGVjIDg+Ji0KCiMgV3JpdGUgb3V0IHRoZSBkZWJ1Zy1ob29rcyBhcmdzLgplY2hvICIiID4gL3RtcC9qdWp1LXVuaXQtbW9uZ29kYi0xLWRlYnVnLWhvb2tzCgojIExvY2sgdGhlIGp1anUtPHVuaXQ+LWRlYnVnLWV4aXQgbG9ja2ZpbGUuCmZsb2NrIC1uIDkgfHwgZXhpdCAxCgojIFdhaXQgZm9yIHRtdXggdG8gYmUgaW5zdGFsbGVkLgp3aGlsZSBbICEgLWYgL3Vzci9iaW4vdG11eCBdOyBkbwogICAgc2xlZXAgMQpkb25lCgppZiBbICEgLWYgfi8udG11eC5jb25mIF07IHRoZW4KICAgICAgICBpZiBbIC1mIC91c3Ivc2hhcmUvYnlvYnUvcHJvZmlsZXMvdG11eCBdOyB0aGVuCiAgICAgICAgICAgICAgICAjIFVzZSBieW9idS90bXV4IHByb2ZpbGUgZm9yIGZhbWlsaWFyIGtleWJpbmRpbmdzIGFuZCBicmFuZGluZwogICAgICAgICAgICAgICAgZWNobyAic291cmNlLWZpbGUgL3Vzci9zaGFyZS9ieW9idS9wcm9maWxlcy90bXV4IiA+IH4vLnRtdXguY29uZgogICAgICAgIGVsc2UKICAgICAgICAgICAgICAgICMgT3RoZXJ3aXNlLCB1c2UgdGhlIGxlZ2FjeSBqdWp1L3RtdXggY29uZmlndXJhdGlvbgogICAgICAgICAgICAgICAgY2F0ID4gfi8udG11eC5jb25mIDw8RU5ECgojIFN0YXR1cyBiYXIKc2V0LW9wdGlvbiAtZyBzdGF0dXMtYmcgYmxhY2sKc2V0LW9wdGlvbiAtZyBzdGF0dXMtZmcgd2hpdGUKCnNldC13aW5kb3ctb3B0aW9uIC1nIHdpbmRvdy1zdGF0dXMtY3VycmVudC1iZyByZWQKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWF0dHIgYnJpZ2h0CgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1yaWdodCAnJwoKIyBQYW5lcwpzZXQtb3B0aW9uIC1nIHBhbmUtYm9yZGVyLWZnIHdoaXRlCnNldC1vcHRpb24gLWcgcGFuZS1hY3RpdmUtYm9yZGVyLWZnIHdoaXRlCgojIE1vbml0b3IgYWN0aXZpdHkgb24gd2luZG93cwpzZXQtd2luZG93LW9wdGlvbiAtZyBtb25pdG9yLWFjdGl2aXR5IG9uCgojIFNjcmVlbiBiaW5kaW5ncywgc2luY2UgcGVvcGxlIGFyZSBtb3JlIGZhbWlsaWFyIHdpdGggdGhhdC4Kc2V0LW9wdGlvbiAtZyBwcmVmaXggQy1hCmJpbmQgQy1hIGxhc3Qtd2luZG93CmJpbmQgYSBzZW5kLWtleSBDLWEKCmJpbmQgfCBzcGxpdC13aW5kb3cgLWgKYmluZCAtIHNwbGl0LXdpbmRvdyAtdgoKIyBGaXggQ1RSTC1QR1VQL1BHRE9XTiBmb3IgdmltCnNldC13aW5kb3ctb3B0aW9uIC1nIHh0ZXJtLWtleXMgb24KCiMgUHJldmVudCBFU0Mga2V5IGZyb20gYWRkaW5nIGRlbGF5IGFuZCBicmVha2luZyBWaW0ncyBFU0MgPiBhcnJvdyBrZXkKc2V0LW9wdGlvbiAtcyBlc2NhcGUtdGltZSAwCgpFTkQKICAgICAgICBmaQpmaQoKKAogICAgIyBDbG9zZSB0aGUgaW5oZXJpdGVkIGxvY2sgRkQsIG9yIHRtdXggd2lsbCBrZWVwIGl0IG9wZW4uCiAgICBleGVjIDk+Ji0KICAgICMgVGhlIGJlYXV0eSBiZWxvdyBpcyBhIHdvcmthcm91bmQgZm9yIGEgYnVnIGluIHRtdXggKDEuNSBpbiBPbmVpcmljKSBvcgogICAgIyBlcG9sbCB0aGF0IGRvZXNuJ3Qgc3VwcG9ydCAvZGV2L251bGwgb3Igd2hhdGV2ZXIuICBXaXRob3V0IGl0IHRoZQogICAgIyBjb21tYW5kIGhhbmdzLgogICAgdG11eCBuZXctc2Vzc2lvbiAtZCAtcyBtb25nb2RiLzEgMj4mMSB8IGNhdCA+IC9kZXYvbnVsbCB8fCB0cnVlCiAgICBleGVjIHRtdXggYXR0YWNoIC10IG1vbmdvZGIvMQopCikgOT4vdG1wL2p1anUtdW5pdC1tb25nb2RiLTEtZGVidWctaG9va3MtZXhpdAopIDg+L3RtcC9qdWp1LXVuaXQtbW9uZ29kYi0xLWRlYnVnLWhvb2tzCmV4aXQgJD8K | base64 -d > $F; . $F'\n"),
		"",
	},
	// "*" is a valid hook name: it means hook everything.
	{
		[]string{"mysql/0", "*"},
		0,
		".*\n",
		"",
	},
	// "*" mixed with named hoooks is equivalent to "*".
	{
		[]string{"mysql/0", "*", "relation-get"},
		0,
		".*\n",
		"",
	},
	// Multiple named hooks may be specified.
	{
		[]string{"mysql/0", "start", "stop"},
		0,
		".*\n",
		"",
	},
	// Relation hooks have the relation name prefixed.
	{
		[]string{"mysql/0", "juju-info-relation-joined"},
		0,
		".*\n",
		"",
	},
	// Invalid unit syntax
	{
		[]string{"mysql"},
		1,
		"",
		`error: "mysql" is not a valid unit name` + "\n",
	},
	// Invalid unit
	{
		[]string{"nonexistant/123"},
		1,
		"",
		`error: unit "nonexistant/123" not found` + "\n",
	},
	// Invalid hook
	{
		[]string{"mysql/0", "invalid-hook"},
		1,
		"",
		`error: unit "mysql/0" does not contain hook "invalid-hook"` + "\n",
	},
}

func (s *DebugHooksSuite) TestDebugHooksCommand(c *C) {
	m := s.makeMachines(3, c)
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	srv, err := s.State.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	s.addUnit(srv, m[0], c)

	srv, err = s.State.AddService("mongodb", dummy)
	c.Assert(err, IsNil)
	s.addUnit(srv, m[1], c)
	s.addUnit(srv, m[2], c)

	for _, t := range debugHooksTests {
		c.Logf("testing juju debug-hooks %s", t.args)
		ctx := coretesting.Context(c)
		code := cmd.Main(&DebugHooksCommand{}, ctx, t.args)
		c.Check(code, Equals, t.code)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), Matches, t.stderr)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), Matches, t.result)
	}
}
