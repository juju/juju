// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/testing"
)

type mockConfig struct {
	agent.Config
	tag               names.Tag
	datadir           string
	logdir            string
	upgradedToVersion version.Number
	jobs              []model.MachineJob
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func (mock *mockConfig) DataDir() string {
	return mock.datadir
}

func (mock *mockConfig) LogDir() string {
	return mock.logdir
}

func (mock *mockConfig) Jobs() []model.MachineJob {
	return mock.jobs
}

func (mock *mockConfig) UpgradedToVersion() version.Number {
	return mock.upgradedToVersion
}

func (mock *mockConfig) WriteUpgradedToVersion(newVersion version.Number) error {
	mock.upgradedToVersion = newVersion
	return nil
}

func (mock *mockConfig) Model() names.ModelTag {
	return testing.ModelTag
}

func (mock *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mock *mockConfig) CACert() string {
	return testing.CACert
}

func (mock *mockConfig) Value(_ string) string {
	return ""
}

func agentConfig(tag names.Tag, datadir, logdir string) agent.Config {
	return &mockConfig{tag: tag, datadir: datadir, logdir: logdir}
}

// assertContains asserts a needle is contained within haystack
func assertContains(c *gc.C, haystack []string, needle string) {
	c.Assert(contains(haystack, needle), jc.IsTrue)
}

// assertNotContains asserts a needle is not contained within haystack
func assertNotContains(c *gc.C, haystack []string, needle string) {
	c.Assert(contains(haystack, needle), gc.Not(jc.IsTrue))
}

func contains(haystack []string, needle string) bool {
	for _, e := range haystack {
		if e == needle {
			return true
		}
	}
	return false
}

type fakeMonitorStatePurger struct {
	purgeCallCount int
}

func (p *fakeMonitorStatePurger) PurgeState(names.Tag) error {
	p.purgeCallCount++
	return nil
}
