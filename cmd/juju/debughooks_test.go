// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"regexp"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	coretesting "launchpad.net/juju-core/testing"
)

var _ = gc.Suite(&DebugHooksSuite{})

type DebugHooksSuite struct {
	SSHCommonSuite
}

const debugHooksArgs = sshArgs

var debugHooksTests = []struct {
	info   string
	args   []string
	code   int
	result string
	stderr string
}{{
	args:   []string{"mysql/0"},
	result: regexp.QuoteMeta(debugHooksArgs + "ubuntu@dummyenv-0.dns sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKAojIExvY2sgdGhlIGp1anUtPHVuaXQ+LWRlYnVnIGxvY2tmaWxlLgpmbG9jayAtbiA4IHx8IChlY2hvICJGYWlsZWQgdG8gYWNxdWlyZSAvdG1wL2p1anUtdW5pdC1teXNxbC0wLWRlYnVnLWhvb2tzOiB1bml0IGlzIGFscmVhZHkgYmVpbmcgZGVidWdnZWQiIDI+JjE7IGV4aXQgMSkKKAojIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KZXhlYyA4PiYtCgojIFdyaXRlIG91dCB0aGUgZGVidWctaG9va3MgYXJncy4KZWNobyAiZTMwSyIgfCBiYXNlNjQgLWQgPiAvdG1wL2p1anUtdW5pdC1teXNxbC0wLWRlYnVnLWhvb2tzCgojIExvY2sgdGhlIGp1anUtPHVuaXQ+LWRlYnVnLWV4aXQgbG9ja2ZpbGUuCmZsb2NrIC1uIDkgfHwgZXhpdCAxCgojIFdhaXQgZm9yIHRtdXggdG8gYmUgaW5zdGFsbGVkLgp3aGlsZSBbICEgLWYgL3Vzci9iaW4vdG11eCBdOyBkbwogICAgc2xlZXAgMQpkb25lCgppZiBbICEgLWYgfi8udG11eC5jb25mIF07IHRoZW4KICAgICAgICBpZiBbIC1mIC91c3Ivc2hhcmUvYnlvYnUvcHJvZmlsZXMvdG11eCBdOyB0aGVuCiAgICAgICAgICAgICAgICAjIFVzZSBieW9idS90bXV4IHByb2ZpbGUgZm9yIGZhbWlsaWFyIGtleWJpbmRpbmdzIGFuZCBicmFuZGluZwogICAgICAgICAgICAgICAgZWNobyAic291cmNlLWZpbGUgL3Vzci9zaGFyZS9ieW9idS9wcm9maWxlcy90bXV4IiA+IH4vLnRtdXguY29uZgogICAgICAgIGVsc2UKICAgICAgICAgICAgICAgICMgT3RoZXJ3aXNlLCB1c2UgdGhlIGxlZ2FjeSBqdWp1L3RtdXggY29uZmlndXJhdGlvbgogICAgICAgICAgICAgICAgY2F0ID4gfi8udG11eC5jb25mIDw8RU5ECiAgICAgICAgICAgICAgICAKIyBTdGF0dXMgYmFyCnNldC1vcHRpb24gLWcgc3RhdHVzLWJnIGJsYWNrCnNldC1vcHRpb24gLWcgc3RhdHVzLWZnIHdoaXRlCgpzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYmcgcmVkCnNldC13aW5kb3ctb3B0aW9uIC1nIHdpbmRvdy1zdGF0dXMtY3VycmVudC1hdHRyIGJyaWdodAoKc2V0LW9wdGlvbiAtZyBzdGF0dXMtcmlnaHQgJycKCiMgUGFuZXMKc2V0LW9wdGlvbiAtZyBwYW5lLWJvcmRlci1mZyB3aGl0ZQpzZXQtb3B0aW9uIC1nIHBhbmUtYWN0aXZlLWJvcmRlci1mZyB3aGl0ZQoKIyBNb25pdG9yIGFjdGl2aXR5IG9uIHdpbmRvd3MKc2V0LXdpbmRvdy1vcHRpb24gLWcgbW9uaXRvci1hY3Rpdml0eSBvbgoKIyBTY3JlZW4gYmluZGluZ3MsIHNpbmNlIHBlb3BsZSBhcmUgbW9yZSBmYW1pbGlhciB3aXRoIHRoYXQuCnNldC1vcHRpb24gLWcgcHJlZml4IEMtYQpiaW5kIEMtYSBsYXN0LXdpbmRvdwpiaW5kIGEgc2VuZC1rZXkgQy1hCgpiaW5kIHwgc3BsaXQtd2luZG93IC1oCmJpbmQgLSBzcGxpdC13aW5kb3cgLXYKCiMgRml4IENUUkwtUEdVUC9QR0RPV04gZm9yIHZpbQpzZXQtd2luZG93LW9wdGlvbiAtZyB4dGVybS1rZXlzIG9uCgojIFByZXZlbnQgRVNDIGtleSBmcm9tIGFkZGluZyBkZWxheSBhbmQgYnJlYWtpbmcgVmltJ3MgRVNDID4gYXJyb3cga2V5CnNldC1vcHRpb24gLXMgZXNjYXBlLXRpbWUgMAoKRU5ECiAgICAgICAgZmkKZmkKCigKICAgICMgQ2xvc2UgdGhlIGluaGVyaXRlZCBsb2NrIEZELCBvciB0bXV4IHdpbGwga2VlcCBpdCBvcGVuLgogICAgZXhlYyA5PiYtCiAgICBleGVjIHRtdXggbmV3LXNlc3Npb24gLXMgbXlzcWwvMAopCikgOT4vdG1wL2p1anUtdW5pdC1teXNxbC0wLWRlYnVnLWhvb2tzLWV4aXQKKSA4Pi90bXAvanVqdS11bml0LW15c3FsLTAtZGVidWctaG9va3MKZXhpdCAkPwo= | base64 -d > $F; . $F'\n"),
}, {
	args:   []string{"mongodb/1"},
	result: regexp.QuoteMeta(debugHooksArgs + "ubuntu@dummyenv-2.dns sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKAojIExvY2sgdGhlIGp1anUtPHVuaXQ+LWRlYnVnIGxvY2tmaWxlLgpmbG9jayAtbiA4IHx8IChlY2hvICJGYWlsZWQgdG8gYWNxdWlyZSAvdG1wL2p1anUtdW5pdC1tb25nb2RiLTEtZGVidWctaG9va3M6IHVuaXQgaXMgYWxyZWFkeSBiZWluZyBkZWJ1Z2dlZCIgMj4mMTsgZXhpdCAxKQooCiMgQ2xvc2UgdGhlIGluaGVyaXRlZCBsb2NrIEZELCBvciB0bXV4IHdpbGwga2VlcCBpdCBvcGVuLgpleGVjIDg+Ji0KCiMgV3JpdGUgb3V0IHRoZSBkZWJ1Zy1ob29rcyBhcmdzLgplY2hvICJlMzBLIiB8IGJhc2U2NCAtZCA+IC90bXAvanVqdS11bml0LW1vbmdvZGItMS1kZWJ1Zy1ob29rcwoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1Zy1leGl0IGxvY2tmaWxlLgpmbG9jayAtbiA5IHx8IGV4aXQgMQoKIyBXYWl0IGZvciB0bXV4IHRvIGJlIGluc3RhbGxlZC4Kd2hpbGUgWyAhIC1mIC91c3IvYmluL3RtdXggXTsgZG8KICAgIHNsZWVwIDEKZG9uZQoKaWYgWyAhIC1mIH4vLnRtdXguY29uZiBdOyB0aGVuCiAgICAgICAgaWYgWyAtZiAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXggXTsgdGhlbgogICAgICAgICAgICAgICAgIyBVc2UgYnlvYnUvdG11eCBwcm9maWxlIGZvciBmYW1pbGlhciBrZXliaW5kaW5ncyBhbmQgYnJhbmRpbmcKICAgICAgICAgICAgICAgIGVjaG8gInNvdXJjZS1maWxlIC91c3Ivc2hhcmUvYnlvYnUvcHJvZmlsZXMvdG11eCIgPiB+Ly50bXV4LmNvbmYKICAgICAgICBlbHNlCiAgICAgICAgICAgICAgICAjIE90aGVyd2lzZSwgdXNlIHRoZSBsZWdhY3kganVqdS90bXV4IGNvbmZpZ3VyYXRpb24KICAgICAgICAgICAgICAgIGNhdCA+IH4vLnRtdXguY29uZiA8PEVORAogICAgICAgICAgICAgICAgCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgooCiAgICAjIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KICAgIGV4ZWMgOT4mLQogICAgZXhlYyB0bXV4IG5ldy1zZXNzaW9uIC1zIG1vbmdvZGIvMQopCikgOT4vdG1wL2p1anUtdW5pdC1tb25nb2RiLTEtZGVidWctaG9va3MtZXhpdAopIDg+L3RtcC9qdWp1LXVuaXQtbW9uZ29kYi0xLWRlYnVnLWhvb2tzCmV4aXQgJD8K | base64 -d > $F; . $F'\n"),
}, {
	info:   `"*" is a valid hook name: it means hook everything`,
	args:   []string{"mysql/0", "*"},
	result: ".*\n",
}, {
	info:   `"*" mixed with named hooks is equivalent to "*"`,
	args:   []string{"mysql/0", "*", "relation-get"},
	result: ".*\n",
}, {
	info:   `multiple named hooks may be specified`,
	args:   []string{"mysql/0", "start", "stop"},
	result: ".*\n",
}, {
	info:   `relation hooks have the relation name prefixed`,
	args:   []string{"mysql/0", "juju-info-relation-joined"},
	result: ".*\n",
}, {
	info:   `invalid unit syntax`,
	args:   []string{"mysql"},
	code:   2,
	stderr: `error: "mysql" is not a valid unit name` + "\n",
}, {
	info:   `invalid unit`,
	args:   []string{"nonexistent/123"},
	code:   1,
	stderr: `error: unit "nonexistent/123" not found` + "\n",
}, {
	info:   `invalid hook`,
	args:   []string{"mysql/0", "invalid-hook"},
	code:   1,
	stderr: `error: unit "mysql/0" does not contain hook "invalid-hook"` + "\n",
}}

func (s *DebugHooksSuite) TestDebugHooksCommand(c *gc.C) {
	machines := s.makeMachines(3, c, true)
	dummy := s.AddTestingCharm(c, "dummy")
	srv := s.AddTestingService(c, "mysql", dummy)
	s.addUnit(srv, machines[0], c)

	srv = s.AddTestingService(c, "mongodb", dummy)
	s.addUnit(srv, machines[1], c)
	s.addUnit(srv, machines[2], c)

	for i, t := range debugHooksTests {
		c.Logf("test %d: %s\n\t%s\n", i, t.info, t.args)
		ctx := coretesting.Context(c)
		code := cmd.Main(&DebugHooksCommand{}, ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, t.stderr)
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), gc.Matches, t.result)
	}
}
