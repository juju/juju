// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"regexp"
	"runtime"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	jujussh "github.com/juju/juju/network/ssh"
)

var _ = gc.Suite(&DebugHooksSuite{})

type DebugHooksSuite struct {
	SSHCommonSuite
}

var debugHooksTests = []struct {
	info        string
	args        []string
	hostChecker jujussh.ReachableChecker
	forceAPIv1  bool
	error       string
	expected    *argsSpec
}{{
	info:        "literal script (api v1: unit name w/o hook or proxy)",
	args:        []string{"mysql/0"},
	hostChecker: validAddresses("0.private", "0.public"),
	forceAPIv1:  true,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		args:            "ubuntu@0.public sudo /bin/bash -c 'F=$(mktemp); echo IyEvYmluL2Jhc2gKKApjbGVhbnVwX29uX2V4aXQoKSAKeyAKCWVjaG8gIkNsZWFuaW5nIHVwIHRoZSBkZWJ1ZyBzZXNzaW9uIgoJdG11eCBraWxsLXNlc3Npb24gLXQgbXlzcWwvMDsgCn0KdHJhcCBjbGVhbnVwX29uX2V4aXQgRVhJVAoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1ZyBsb2NrZmlsZS4KZmxvY2sgLW4gOCB8fCAoCgllY2hvICJGb3VuZCBleGlzdGluZyBkZWJ1ZyBzZXNzaW9ucywgYXR0ZW1wdGluZyB0byByZWNvbm5lY3QiIDI+JjEKCWV4ZWMgdG11eCBhdHRhY2gtc2Vzc2lvbiAtdCBteXNxbC8wCglleGl0ICQ/CgkpCigKIyBDbG9zZSB0aGUgaW5oZXJpdGVkIGxvY2sgRkQsIG9yIHRtdXggd2lsbCBrZWVwIGl0IG9wZW4uCmV4ZWMgOD4mLQoKIyBXcml0ZSBvdXQgdGhlIGRlYnVnLWhvb2tzIGFyZ3MuCmVjaG8gImUzMEsiIHwgYmFzZTY0IC1kID4gL3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcwoKIyBMb2NrIHRoZSBqdWp1LTx1bml0Pi1kZWJ1Zy1leGl0IGxvY2tmaWxlLgpmbG9jayAtbiA5IHx8IGV4aXQgMQoKIyBXYWl0IGZvciB0bXV4IHRvIGJlIGluc3RhbGxlZC4Kd2hpbGUgWyAhIC1mIC91c3IvYmluL3RtdXggXTsgZG8KICAgIHNsZWVwIDEKZG9uZQoKaWYgWyAhIC1mIH4vLnRtdXguY29uZiBdOyB0aGVuCiAgICAgICAgaWYgWyAtZiAvdXNyL3NoYXJlL2J5b2J1L3Byb2ZpbGVzL3RtdXggXTsgdGhlbgogICAgICAgICAgICAgICAgIyBVc2UgYnlvYnUvdG11eCBwcm9maWxlIGZvciBmYW1pbGlhciBrZXliaW5kaW5ncyBhbmQgYnJhbmRpbmcKICAgICAgICAgICAgICAgIGVjaG8gInNvdXJjZS1maWxlIC91c3Ivc2hhcmUvYnlvYnUvcHJvZmlsZXMvdG11eCIgPiB+Ly50bXV4LmNvbmYKICAgICAgICBlbHNlCiAgICAgICAgICAgICAgICAjIE90aGVyd2lzZSwgdXNlIHRoZSBsZWdhY3kganVqdS90bXV4IGNvbmZpZ3VyYXRpb24KICAgICAgICAgICAgICAgIGNhdCA+IH4vLnRtdXguY29uZiA8PEVORAogICAgICAgICAgICAgICAgCiMgU3RhdHVzIGJhcgpzZXQtb3B0aW9uIC1nIHN0YXR1cy1iZyBibGFjawpzZXQtb3B0aW9uIC1nIHN0YXR1cy1mZyB3aGl0ZQoKc2V0LXdpbmRvdy1vcHRpb24gLWcgd2luZG93LXN0YXR1cy1jdXJyZW50LWJnIHJlZApzZXQtd2luZG93LW9wdGlvbiAtZyB3aW5kb3ctc3RhdHVzLWN1cnJlbnQtYXR0ciBicmlnaHQKCnNldC1vcHRpb24gLWcgc3RhdHVzLXJpZ2h0ICcnCgojIFBhbmVzCnNldC1vcHRpb24gLWcgcGFuZS1ib3JkZXItZmcgd2hpdGUKc2V0LW9wdGlvbiAtZyBwYW5lLWFjdGl2ZS1ib3JkZXItZmcgd2hpdGUKCiMgTW9uaXRvciBhY3Rpdml0eSBvbiB3aW5kb3dzCnNldC13aW5kb3ctb3B0aW9uIC1nIG1vbml0b3ItYWN0aXZpdHkgb24KCiMgU2NyZWVuIGJpbmRpbmdzLCBzaW5jZSBwZW9wbGUgYXJlIG1vcmUgZmFtaWxpYXIgd2l0aCB0aGF0LgpzZXQtb3B0aW9uIC1nIHByZWZpeCBDLWEKYmluZCBDLWEgbGFzdC13aW5kb3cKYmluZCBhIHNlbmQta2V5IEMtYQoKYmluZCB8IHNwbGl0LXdpbmRvdyAtaApiaW5kIC0gc3BsaXQtd2luZG93IC12CgojIEZpeCBDVFJMLVBHVVAvUEdET1dOIGZvciB2aW0Kc2V0LXdpbmRvdy1vcHRpb24gLWcgeHRlcm0ta2V5cyBvbgoKIyBQcmV2ZW50IEVTQyBrZXkgZnJvbSBhZGRpbmcgZGVsYXkgYW5kIGJyZWFraW5nIFZpbSdzIEVTQyA+IGFycm93IGtleQpzZXQtb3B0aW9uIC1zIGVzY2FwZS10aW1lIDAKCkVORAogICAgICAgIGZpCmZpCgooCiAgICAjIENsb3NlIHRoZSBpbmhlcml0ZWQgbG9jayBGRCwgb3IgdG11eCB3aWxsIGtlZXAgaXQgb3Blbi4KICAgIGV4ZWMgOT4mLQogICAgaWYgISB0bXV4IGhhcy1zZXNzaW9uIC10IG15c3FsLzA7IHRoZW4KCQl0bXV4IG5ldy1zZXNzaW9uIC1kIC1zIG15c3FsLzAKCWZpCgljbGllbnRfY291bnQ9JCh0bXV4IGxpc3QtY2xpZW50cyB8IHdjIC1sKQoJaWYgWyAkY2xpZW50X2NvdW50IC1nZSAxIF07IHRoZW4KCQlzZXNzaW9uX25hbWU9bXlzcWwvMCItIiRjbGllbnRfY250CgkJZXhlYyB0bXV4IG5ldy1zZXNzaW9uIC1kIC10IG15c3FsLzAgLXMgJHNlc3Npb25fbmFtZQoJCWV4ZWMgdG11eCBhdHRhY2gtc2Vzc2lvbiAtdCAkc2Vzc2lvbl9uYW1lIFw7IHNldC1vcHRpb24gZGVzdHJveS11bmF0dGFjaGVkCgllbHNlCgkgICAgZXhlYyB0bXV4IGF0dGFjaC1zZXNzaW9uIC10IG15c3FsLzAKCWZpCikKKSA5Pi90bXAvanVqdS11bml0LW15c3FsLTAtZGVidWctaG9va3MtZXhpdAopIDg+L3RtcC9qdWp1LXVuaXQtbXlzcWwtMC1kZWJ1Zy1ob29rcwpleGl0ICQ/Cg== | base64 -d > $F; . $F'",
	},
}, {
	info:        "unit name without hook (api v1)",
	args:        []string{"mysql/0"},
	hostChecker: validAddresses("0.private", "0.public"),
	forceAPIv1:  true,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		argsMatch:       `ubuntu@0\.public sudo /bin/bash .+`,
	},
}, {
	info:        "unit name without hook (api v2)",
	args:        []string{"mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	forceAPIv1:  false,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) sudo .+`, // can be any of the 3
	},
}, {
	info:        "proxy (api v1)",
	args:        []string{"--proxy=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public"),
	forceAPIv1:  true,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		argsMatch:       `ubuntu@0\.private sudo /bin/bash .+`,
	},
}, {
	info:        "proxy (api v2)",
	args:        []string{"--proxy=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	forceAPIv1:  false,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		withProxy:       true,
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) sudo .+`, // can be any of the 3
	},
}, {
	info:        "pty enabled",
	args:        []string{"--pty=true", "mysql/0"},
	hostChecker: validAddresses("0.private", "0.public", "0.1.2.3"), // set by setAddresses() and setLinkLayerDevicesAddresses()
	forceAPIv1:  false,
	expected: &argsSpec{
		hostKeyChecking: "yes",
		knownHosts:      "0",
		enablePty:       true,
		argsMatch:       `ubuntu@0\.(private|public|1\.2\.3) sudo .+`, // can be any of the 3
	},
}, {
	info:        `"*" is a valid hook name: it means hook everything`,
	args:        []string{"mysql/0", "*"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:        `"*" mixed with named hooks is equivalent to "*"`,
	args:        []string{"mysql/0", "*", "relation-get"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:        `multiple named hooks may be specified`,
	args:        []string{"mysql/0", "start", "stop"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
}, {
	info:        `relation hooks have the relation name prefixed`,
	args:        []string{"mysql/0", "juju-info-relation-joined"},
	hostChecker: validAddresses("0.public"),
	expected:    nil,
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
	error: `unit "mysql/0" contains neither hook nor action "invalid-hook", valid actions are [anotherfakeaction fakeaction] and valid hooks are [collect-metrics config-changed install juju-info-relation-broken juju-info-relation-changed juju-info-relation-created juju-info-relation-departed juju-info-relation-joined leader-deposed leader-elected leader-settings-changed meter-status-changed metrics-client-relation-broken metrics-client-relation-changed metrics-client-relation-created metrics-client-relation-departed metrics-client-relation-joined post-series-upgrade pre-series-upgrade remove server-admin-relation-broken server-admin-relation-changed server-admin-relation-created server-admin-relation-departed server-admin-relation-joined server-relation-broken server-relation-changed server-relation-created server-relation-departed server-relation-joined start stop update-status upgrade-charm]`,
}, {
	info:  `no args at all`,
	args:  nil,
	error: `no unit name specified`,
}}

func (s *DebugHooksSuite) TestDebugHooksCommand(c *gc.C) {
	//TODO(bogdanteleaga): Fix once debughooks are supported on windows
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping on windows for now")
	}

	s.setupModel(c)

	for i, t := range debugHooksTests {
		c.Logf("test %d: %s\n\t%s\n", i, t.info, t.args)

		s.setHostChecker(t.hostChecker)
		s.setForceAPIv1(t.forceAPIv1)

		ctx, err := cmdtesting.RunCommand(c, newDebugHooksCommand(s.hostChecker), t.args...)
		if t.error != "" {
			c.Check(err, gc.ErrorMatches, regexp.QuoteMeta(t.error))
		} else {
			c.Check(err, jc.ErrorIsNil)
			if t.expected != nil {
				t.expected.check(c, cmdtesting.Stdout(ctx))
			}
		}
	}
}

func (s *DebugHooksSuite) TestDebugHooksArgFormatting(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Skipping on windows for now")
	}
	s.setupModel(c)
	s.setHostChecker(validAddresses("0.public"))
	ctx, err := cmdtesting.RunCommand(c, newDebugHooksCommand(s.hostChecker),
		"mysql/0", "install", "start")
	c.Check(err, jc.ErrorIsNil)
	base64Regex := regexp.MustCompile("echo ([A-Za-z0-9+/]+=*) \\| base64")
	c.Check(err, jc.ErrorIsNil)
	rawContent := base64Regex.FindString(cmdtesting.Stdout(ctx))
	c.Check(rawContent, gc.Not(gc.Equals), "")
	// Strip off the "echo " and " | base64"
	prefix := "echo "
	suffix := " | base64"
	c.Check(strings.HasPrefix(rawContent, prefix), jc.IsTrue)
	c.Check(strings.HasSuffix(rawContent, suffix), jc.IsTrue)
	b64content := rawContent[len(prefix) : len(rawContent)-len(suffix)]
	scriptContent, err := base64.StdEncoding.DecodeString(b64content)
	c.Assert(string(scriptContent), gc.Not(gc.Equals), "")
	// Inside the script is another base64 encoded string telling us the debug-hook args
	debugArgsRegex := regexp.MustCompile(`echo "([A-Z-a-z0-9+/]+=*)" \| base64.*-debug-hooks`)
	debugArgsCommand := debugArgsRegex.FindString(string(scriptContent))
	debugArgsB64 := debugArgsCommand[len(`echo "`):strings.Index(debugArgsCommand, `" | base64`)]
	yamlContent, err := base64.StdEncoding.DecodeString(string(debugArgsB64))
	var args map[string]interface{}
	err = goyaml.Unmarshal(yamlContent, &args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(args, gc.DeepEquals, map[string]interface{}{
		"hooks": []interface{}{"install", "start"},
	})
}
