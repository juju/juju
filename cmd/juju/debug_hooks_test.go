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
)

var _ = Suite(&DebugHooksSuite{})

type DebugHooksSuite struct {
	SSHCommonSuite
}

const debugHooksArgs = `-l ubuntu -t ` + commonArgs

var debugHooksTests = []struct {
	args   []string
	result string
}{
	{
		[]string{"mysql/0"},
		debugHooksArgs + "dummyenv-0.dns -- sudo /bin/bash -c 'F=$(mktemp); echo CiMgV2FpdCBmb3IgdG11eCB0byBiZSBpbnN0YWxsZWQuCndoaWxlIFsgISAtZiAvdXNyL2Jpbi90bXV4IF07IGRvCiAgICBzbGVlcCAxCmRvbmUKCmlmIFsgISAtZiB+Ly50bXV4LmNvbmYgXTsgdGhlbgogICAgICAgIGlmIFsgLWYgL3Vzci9zaGFyZS9ieW9idS9wcm9maWxlcy90bXV4IF07IHRoZW4KICAgICAgICAgICAgICAgICMgVXNlIGJ5b2J1L3RtdXggcHJvZmlsZSBmb3IgZmFtaWxpYXIga2V5YmluZGluZ3MgYW5kIGJyYW5kaW5nCiAgICAgICAgICAgICAgICBlY2hvICJzb3VyY2UtZmlsZSAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXgiID4gfi8udG11eC5jb25mCiAgICAgICAgZWxzZQogICAgICAgICAgICAgICAgIyBPdGhlcndpc2UsIHVzZSB0aGUgbGVnYWN5IGp1anUvdG11eCBjb25maWd1cmF0aW9uCiAgICAgICAgICAgICAgICBjYXQgPiB+Ly50bXV4LmNvbmYgPDxFTkQKCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgojIFRoZSBiZWF1dHkgYmVsb3cgaXMgYSB3b3JrYXJvdW5kIGZvciBhIGJ1ZyBpbiB0bXV4ICgxLjUgaW4gT25laXJpYykgb3IKIyBlcG9sbCB0aGF0IGRvZXNuJ3Qgc3VwcG9ydCAvZGV2L251bGwgb3Igd2hhdGV2ZXIuICBXaXRob3V0IGl0IHRoZQojIGNvbW1hbmQgaGFuZ3MuCnRtdXggbmV3LXNlc3Npb24gLWQgLXMgbXlzcWwvMCAyPiYxIHwgY2F0ID4gL2Rldi9udWxsIHx8IHRydWUKdG11eCBhdHRhY2ggLXQgbXlzcWwvMAo= | base64 -d > $F; . $F'\n",
	},
	{
		[]string{"mongodb/1"},
		debugHooksArgs + "dummyenv-2.dns -- sudo /bin/bash -c 'F=$(mktemp); echo CiMgV2FpdCBmb3IgdG11eCB0byBiZSBpbnN0YWxsZWQuCndoaWxlIFsgISAtZiAvdXNyL2Jpbi90bXV4IF07IGRvCiAgICBzbGVlcCAxCmRvbmUKCmlmIFsgISAtZiB+Ly50bXV4LmNvbmYgXTsgdGhlbgogICAgICAgIGlmIFsgLWYgL3Vzci9zaGFyZS9ieW9idS9wcm9maWxlcy90bXV4IF07IHRoZW4KICAgICAgICAgICAgICAgICMgVXNlIGJ5b2J1L3RtdXggcHJvZmlsZSBmb3IgZmFtaWxpYXIga2V5YmluZGluZ3MgYW5kIGJyYW5kaW5nCiAgICAgICAgICAgICAgICBlY2hvICJzb3VyY2UtZmlsZSAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXgiID4gfi8udG11eC5jb25mCiAgICAgICAgZWxzZQogICAgICAgICAgICAgICAgIyBPdGhlcndpc2UsIHVzZSB0aGUgbGVnYWN5IGp1anUvdG11eCBjb25maWd1cmF0aW9uCiAgICAgICAgICAgICAgICBjYXQgPiB+Ly50bXV4LmNvbmYgPDxFTkQKCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgojIFRoZSBiZWF1dHkgYmVsb3cgaXMgYSB3b3JrYXJvdW5kIGZvciBhIGJ1ZyBpbiB0bXV4ICgxLjUgaW4gT25laXJpYykgb3IKIyBlcG9sbCB0aGF0IGRvZXNuJ3Qgc3VwcG9ydCAvZGV2L251bGwgb3Igd2hhdGV2ZXIuICBXaXRob3V0IGl0IHRoZQojIGNvbW1hbmQgaGFuZ3MuCnRtdXggbmV3LXNlc3Npb24gLWQgLXMgbW9uZ29kYi8xIDI+JjEgfCBjYXQgPiAvZGV2L251bGwgfHwgdHJ1ZQp0bXV4IGF0dGFjaCAtdCBtb25nb2RiLzEK | base64 -d > $F; . $F'\n",
	},
}

func (s *DebugHooksSuite) TestSSHCommand(c *C) {
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
		c.Check(code, Equals, 0)
		c.Check(ctx.Stderr.(*bytes.Buffer).String(), Equals, "")
		c.Check(ctx.Stdout.(*bytes.Buffer).String(), Equals, t.result)
	}
}
