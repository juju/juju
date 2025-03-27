// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/testing"
)

type mockConfig struct {
	agent.Config
	tag               names.Tag
	datadir           string
	logdir            string
	upgradedToVersion semversion.Number
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

func (mock *mockConfig) UpgradedToVersion() semversion.Number {
	return mock.upgradedToVersion
}

func (mock *mockConfig) WriteUpgradedToVersion(newVersion semversion.Number) error {
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
