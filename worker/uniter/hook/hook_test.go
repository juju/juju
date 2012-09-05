package hook_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/worker/uniter/hook"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type InfoSuite struct{}

var _ = Suite(&InfoSuite{})

var validateTests = []struct {
	info hook.Info
	err  string
}{
	{
		hook.Info{Kind: hook.RelationJoined},
		`"relation-joined" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hook.RelationChanged},
		`"relation-changed" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hook.RelationDeparted},
		`"relation-departed" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hook.Kind("grok")},
		`unknown hook kind "grok"`,
	},
	{hook.Info{Kind: hook.Install}, ""},
	{hook.Info{Kind: hook.Start}, ""},
	{hook.Info{Kind: hook.ConfigChanged}, ""},
	{hook.Info{Kind: hook.UpgradeCharm}, ""},
	{hook.Info{Kind: hook.Stop}, ""},
	{hook.Info{Kind: hook.RelationJoined, RemoteUnit: "x"}, ""},
	{hook.Info{Kind: hook.RelationChanged, RemoteUnit: "x"}, ""},
	{hook.Info{Kind: hook.RelationDeparted, RemoteUnit: "x"}, ""},
	{hook.Info{Kind: hook.RelationBroken}, ""},
}

func (s *InfoSuite) TestValidate(c *C) {
	for i, t := range validateTests {
		c.Logf("test %d", i)
		err := t.info.Validate()
		if t.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}
