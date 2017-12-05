// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"bytes"
	"io"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/runner/context"
)

const (
	formatYaml = iota
	formatJson
)

func bufferBytes(stream io.Writer) []byte {
	return stream.(*bytes.Buffer).Bytes()
}

func bufferString(w io.Writer) string {
	return w.(*bytes.Buffer).String()
}

type ContextSuite struct {
	testing.BaseSuite
}

func (s *ContextSuite) newHookContext(c *gc.C) *mockHookContext {
	return &mockHookContext{}
}

func (s *ContextSuite) GetStatusContext(c *gc.C) commands.Context {
	return s.newHookContext(c)
}

func (s *ContextSuite) GetHookContextWithSettings(c *gc.C, config charm.Settings) commands.Context {
	ctx := s.newHookContext(c)
	ctx.config = config
	return ctx
}

type mockHookContext struct {
	context.HookContext

	status            commands.StatusInfo
	config            charm.Settings
	containerSpec     string
	containerSpecUnit string
}

func (m *mockHookContext) ApplicationStatus() (commands.StatusInfo, error) {
	return m.status, nil
}

func (m *mockHookContext) SetApplicationStatus(appStatus commands.StatusInfo) error {
	m.status = appStatus
	return nil
}

func (m *mockHookContext) ApplicationConfig() (charm.Settings, error) {
	settingsCopy := make(charm.Settings)
	for k, v := range m.config {
		settingsCopy[k] = v
	}
	return settingsCopy, nil
}

func (m *mockHookContext) SetContainerSpec(unitName, spec string) error {
	m.containerSpec = spec
	m.containerSpecUnit = unitName
	return nil
}
