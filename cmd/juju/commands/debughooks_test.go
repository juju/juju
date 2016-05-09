// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&DebugHooksSuite{})

type DebugHooksSuite struct {
	SSHCommonSuite
}

var debugHooksTests = []struct {
	info     string
	args     []string
	error    string
	proxy    bool
	expected *argsSpec
}{{
	info:  "unit name without hook",
	args:  []string{"mysql/0"},
	proxy: true,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		enablePty:       true,
		args:            "ubuntu@0.public sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKApjbGVhbnVwX29uX2V4aXQoKSAKeyAKCWVjaG8gIkNsZWFuaW5nIHVwIHRoZSBkZWJ1ZyBzZXNzaW9uIgoJdG11eCBraWxsLXNlc3Npb24gLXQgbXlzcWwvMDsgCn0KdHJhcCBjbGVhbnVwX29uX2V4aXQgRVhJVAoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1ZyBsb2NrZmlsZS4KZmxvY2sgLW4gOCB8fCAoCgllY2hvICJGb3VuZCBleGlzdGluZyBkZWJ1ZyBzZXNzaW9ucywgYXR0ZW1wdGluZyB0byByZWNvbm5lY3QiIDI+JjEKCWV4ZWMgdG11eCBhdHRhY2gtc2Vzc2lvbiAtdCBteXNxbC8wCglleGl0ICQ/CgkpCigKIyBDbG9zZSB0aGUgaW5oZXJpdGVkIGxvY2sgRkQsIG9yIHRtdXggd2lsbCBrZWVwIGl0IG9wZW4uCmV4ZWMgOD4mLQoKIyBXcml0ZSBvdXQgdGhlIGRlYnVnLWhvb2tzIGFyZ3MuCmVjaG8gImUzMEsiIHwgYmFzZTY0IC1kID4gL3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcwoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1Zy1leGl0IGxvY2tmaWxlLgpmbG9jayAtbiA5IHx8IGV4aXQgMQoKIyBXYWl0IGZvciB0bXV4IHRvIGJlIGluc3RhbGxlZC4Kd2hpbGUgWyAhIC1mIC91c3IvYmluL3RtdXggXTsgZG8KICAgIHNsZWVwIDEKZG9uZQoKaWYgWyAhIC1mIH4vLnRtdXguY29uZiBdOyB0aGVuCiAgICAgICAgaWYgWyAtZiAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXggXTsgdGhlbgogICAgICAgICAgICAgICAgIyBVc2UgYnlvYnUvdG11eCBwcm9maWxlIGZvciBmYW1pbGlhciBrZXliaW5kaW5ncyBhbmQgYnJhbmRpbmcKICAgICAgICAgICAgICAgIGVjaG8gInNvdXJjZS1maWxlIC91c3Ivc2hhcmUvYnlvYnUvcHJvZmlsZXMvdG11eCIgPiB+Ly50bXV4LmNvbmYKICAgICAgICBlbHNlCiAgICAgICAgICAgICAgICAjIE90aGVyd2lzZSwgdXNlIHRoZSBsZWdhY3kganVqdS90bXV4IGNvbmZpZ3VyYXRpb24KICAgICAgICAgICAgICAgIGNhdCA+IH4vLnRtdXguY29uZiA8PEVORAogICAgICAgICAgICAgICAgCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgooCiAgICAjIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KICAgIGV4ZWMgOT4mLQogICAgaWYgISB0bXV4IGhhcy1zZXNzaW9uIC10IG15c3FsLzA7IHRoZW4KCQl0bXV4IG5ldy1zZXNzaW9uIC1kIC1zIG15c3FsLzAKCWZpCgljbGllbnRfY291bnQ9JCh0bXV4IGxpc3QtY2xpZW50cyB8IHdjIC1sKQoJaWYgWyAkY2xpZW50X2NvdW50IC1nZSAxIF07IHRoZW4KCQlzZXNzaW9uX25hbWU9bXlzcWwvMCItIiRjbGllbnRfY250CgkJZXhlYyB0bXV4IG5ldy1zZXNzaW9uIC1kIC10IG15c3FsLzAgLXMgJHNlc3Npb25fbmFtZQoJCWV4ZWMgdG11eCBhdHRhY2gtc2Vzc2lvbiAtdCAkc2Vzc2lvbl9uYW1lIFw7IHNldC1vcHRpb24gZGVzdHJveS11bmF0dGFjaGVkCgllbHNlCgkgICAgZXhlYyB0bXV4IGF0dGFjaC1zZXNzaW9uIC10IG15c3FsLzAKCWZpCikKKSA5Pi90bXAvanVqdS11bml0LW15c3FsLTAtZGVidWctaG9va3MtZXhpdAopIDg+L3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcwpleGl0ICQ/Cg== | base64 -d > $F; . $F'",
	},
}, {
	info: "proxy",
	args: []string{"--proxy=true", "mysql/0"},
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		enablePty:       true,
		withProxy:       true,
		args:            "ubuntu@0.private sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKApjbGVhbnVwX29uX2V4aXQoKSAKeyAKCWVjaG8gIkNsZWFuaW5nIHVwIHRoZSBkZWJ1ZyBzZXNzaW9uIgoJdG11eCBraWxsLXNlc3Npb24gLXQgbXlzcWwvMDsgCn0KdHJhcCBjbGVhbnVwX29uX2V4aXQgRVhJVAoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1ZyBsb2NrZmlsZS4KZmxvY2sgLW4gOCB8fCAoCgllY2hvICJGb3VuZCBleGlzdGluZyBkZWJ1ZyBzZXNzaW9ucywgYXR0ZW1wdGluZyB0byByZWNvbm5lY3QiIDI+JjEKCWV4ZWMgdG11eCBhdHRhY2gtc2Vzc2lvbiAtdCBteXNxbC8wCglleGl0ICQ/CgkpCigKIyBDbG9zZSB0aGUgaW5oZXJpdGVkIGxvY2sgRkQsIG9yIHRtdXggd2lsbCBrZWVwIGl0IG9wZW4uCmV4ZWMgOD4mLQoKIyBXcml0ZSBvdXQgdGhlIGRlYnVnLWhvb2tzIGFyZ3MuCmVjaG8gImUzMEsiIHwgYmFzZTY0IC1kID4gL3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcwoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1Zy1leGl0IGxvY2tmaWxlLgpmbG9jayAtbiA5IHx8IGV4aXQgMQoKIyBXYWl0IGZvciB0bXV4IHRvIGJlIGluc3RhbGxlZC4Kd2hpbGUgWyAhIC1mIC91c3IvYmluL3RtdXggXTsgZG8KICAgIHNsZWVwIDEKZG9uZQoKaWYgWyAhIC1mIH4vLnRtdXguY29uZiBdOyB0aGVuCiAgICAgICAgaWYgWyAtZiAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXggXTsgdGhlbgogICAgICAgICAgICAgICAgIyBVc2UgYnlvYnUvdG11eCBwcm9maWxlIGZvciBmYW1pbGlhciBrZXliaW5kaW5ncyBhbmQgYnJhbmRpbmcKICAgICAgICAgICAgICAgIGVjaG8gInNvdXJjZS1maWxlIC91c3Ivc2hhcmUvYnlvYnUvcHJvZmlsZXMvdG11eCIgPiB+Ly50bXV4LmNvbmYKICAgICAgICBlbHNlCiAgICAgICAgICAgICAgICAjIE90aGVyd2lzZSwgdXNlIHRoZSBsZWdhY3kganVqdS90bXV4IGNvbmZpZ3VyYXRpb24KICAgICAgICAgICAgICAgIGNhdCA+IH4vLnRtdXguY29uZiA8PEVORAogICAgICAgICAgICAgICAgCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgooCiAgICAjIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KICAgIGV4ZWMgOT4mLQogICAgaWYgISB0bXV4IGhhcy1zZXNzaW9uIC10IG15c3FsLzA7IHRoZW4KCQl0bXV4IG5ldy1zZXNzaW9uIC1kIC1zIG15c3FsLzAKCWZpCgljbGllbnRfY291bnQ9JCh0bXV4IGxpc3QtY2xpZW50cyB8IHdjIC1sKQoJaWYgWyAkY2xpZW50X2NvdW50IC1nZSAxIF07IHRoZW4KCQlzZXNzaW9uX25hbWU9bXlzcWwvMCItIiRjbGllbnRfY250CgkJZXhlYyB0bXV4IG5ldy1zZXNzaW9uIC1kIC10IG15c3FsLzAgLXMgJHNlc3Npb25fbmFtZQoJCWV4ZWMgdG11eCBhdHRhY2gtc2Vzc2lvbiAtdCAkc2Vzc2lvbl9uYW1lIFw7IHNldC1vcHRpb24gZGVzdHJveS11bmF0dGFjaGVkCgllbHNlCgkgICAgZXhlYyB0bXV4IGF0dGFjaC1zZXNzaW9uIC10IG15c3FsLzAKCWZpCikKKSA5Pi90bXAvanVqdS11bml0LW15c3FsLTAtZGVidWctaG9va3MtZXhpdAopIDg+L3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcwpleGl0ICQ/Cg== | base64 -d > $F; . $F'",
	},
}, {
	info:     `"*" is a valid hook name: it means hook everything`,
	args:     []string{"mysql/0", "*"},
	expected: nil,
}, {
	info:     `"*" mixed with named hooks is equivalent to "*"`,
	args:     []string{"mysql/0", "*", "relation-get"},
	expected: nil,
}, {
	info:     `multiple named hooks may be specified`,
	args:     []string{"mysql/0", "start", "stop"},
	expected: nil,
}, {
	info:     `relation hooks have the relation name prefixed`,
	args:     []string{"mysql/0", "juju-info-relation-joined"},
	expected: nil,
}, {
	info:  `invalid unit syntax`,
	args:  []string{"mysql"},
	error: `"mysql" is not a valid unit name`,
}, {
	info:  `invalid unit`,
	args:  []string{"nonexistent/123"},
	error: `unit "nonexistent/123" not found`,
}, {
	info:  `invalid hook`,
	args:  []string{"mysql/0", "invalid-hook"},
	error: `unit "mysql/0" does not contain hook "invalid-hook"`,
}}

func (s *DebugHooksSuite) TestDebugHooksCommand(c *gc.C) {
	//TODO(bogdanteleaga): Fix once debughooks are supported on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping on windows for now")
	}

	s.setupModel(c)

	for i, t := range debugHooksTests {
		c.Logf("test %d: %s\n\t%s\n", i, t.info, t.args)

		ctx, err := coretesting.RunCommand(c, newDebugHooksCommand(), t.args...)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, t.error)
		} else {
			c.Check(err, jc.ErrorIsNil)
			if t.expected != nil {
				t.expected.check(c, coretesting.Stdout(ctx))
			}
		}
	}
}
