// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hook_test

import (
	"github.com/juju/charm/v12/hooks"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/testing"
)

type InfoSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&InfoSuite{})

var validateTests = []struct {
	info hook.Info
	err  string
}{
	{
		hook.Info{Kind: hooks.RelationJoined},
		`"relation-joined" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hooks.RelationJoined, RemoteUnit: "foo/0"},
		`"relation-joined" hook has a remote unit but no application`,
	}, {
		hook.Info{Kind: hooks.RelationChanged},
		`"relation-changed" hook requires a remote unit or application`,
	}, {
		hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "foo/0"},
		`"relation-changed" hook has a remote unit but no application`,
	}, {
		hook.Info{Kind: hooks.RelationDeparted},
		`"relation-departed" hook requires a remote unit`,
	}, {
		hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "foo/0"},
		`"relation-departed" hook has a remote unit but no application`,
	}, {
		hook.Info{Kind: hooks.Kind("grok")},
		`unknown hook kind "grok"`,
	}, {
		hook.Info{Kind: hooks.PebbleReady},
		`"pebble-ready" hook requires a workload name`,
	}, {
		hook.Info{Kind: hooks.PreSeriesUpgrade},
		`"pre-series-upgrade" hook requires a target base`,
	}, {
		hook.Info{Kind: hooks.SecretRotate},
		`"secret-rotate" hook requires a secret URI`,
	}, {
		hook.Info{Kind: hooks.SecretExpired, SecretURI: "secret:9m4e2mr0ui3e8a215n4g"},
		`"secret-expired" hook requires a secret revision`,
	}, {
		hook.Info{Kind: hooks.SecretRotate, SecretURI: "foo"},
		`invalid secret URI "foo"`,
	},
	{hook.Info{Kind: hooks.Install}, ""},
	{hook.Info{Kind: hooks.Start}, ""},
	{hook.Info{Kind: hooks.ConfigChanged}, ""},
	{hook.Info{Kind: hooks.CollectMetrics}, ""},
	{hook.Info{Kind: hooks.MeterStatusChanged}, ""},
	{hook.Info{Kind: hooks.Action}, "hooks.Kind Action is deprecated"},
	{hook.Info{Kind: hooks.UpgradeCharm}, ""},
	{hook.Info{Kind: hooks.Stop}, ""},
	{hook.Info{Kind: hooks.Remove}, ""},
	{hook.Info{Kind: hooks.RelationJoined, RemoteUnit: "x/0", RemoteApplication: "x"}, ""},
	{hook.Info{Kind: hooks.RelationChanged, RemoteUnit: "x/0", RemoteApplication: "x"}, ""},
	{hook.Info{Kind: hooks.RelationChanged, RemoteApplication: "x"}, ""},
	{hook.Info{Kind: hooks.RelationDeparted, RemoteUnit: "x/0", RemoteApplication: "x"}, ""},
	{hook.Info{Kind: hooks.RelationBroken}, ""},
	{hook.Info{Kind: hooks.StorageAttached}, `invalid storage ID ""`},
	{hook.Info{Kind: hooks.StorageAttached, StorageId: "data/0"}, ""},
	{hook.Info{Kind: hooks.StorageDetaching, StorageId: "data/0"}, ""},
	{hook.Info{Kind: hooks.PebbleReady, WorkloadName: "gitlab"}, ""},
	{hook.Info{Kind: hooks.PreSeriesUpgrade, MachineUpgradeTarget: "ubuntu@20.04"}, ""},
}

func (s *InfoSuite) TestValidate(c *gc.C) {
	for i, t := range validateTests {
		c.Logf("test %d", i)
		err := t.info.Validate()
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}
