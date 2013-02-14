package hook_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm/hook"
	uhook "launchpad.net/juju-core/worker/uniter/hook"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type InfoSuite struct{}

var _ = Suite(&InfoSuite{})

var validateTests = []struct {
	info uhook.Info
	err  string
}{
	{
		uhook.Info{Kind: hook.RelationJoined},
		`"relation-joined" hook requires a remote unit`,
	}, {
		uhook.Info{Kind: hook.RelationChanged},
		`"relation-changed" hook requires a remote unit`,
	}, {
		uhook.Info{Kind: hook.RelationDeparted},
		`"relation-departed" hook requires a remote unit`,
	}, {
		uhook.Info{Kind: hook.Kind("grok")},
		`unknown hook kind "grok"`,
	},
	{uhook.Info{Kind: hook.Install}, ""},
	{uhook.Info{Kind: hook.Start}, ""},
	{uhook.Info{Kind: hook.ConfigChanged}, ""},
	{uhook.Info{Kind: hook.UpgradeCharm}, ""},
	{uhook.Info{Kind: hook.Stop}, ""},
	{uhook.Info{Kind: hook.RelationJoined, RemoteUnit: "x"}, ""},
	{uhook.Info{Kind: hook.RelationChanged, RemoteUnit: "x"}, ""},
	{uhook.Info{Kind: hook.RelationDeparted, RemoteUnit: "x"}, ""},
	{uhook.Info{Kind: hook.RelationBroken}, ""},
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
