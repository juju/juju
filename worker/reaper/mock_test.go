// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reaper_test

import (
	"sync"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type clientEnviron struct {
	Life                   state.Life
	TimeOfDeath            *time.Time
	UUID                   string
	IsSystem               bool
	HasMachinesAndServices bool
	Removed                bool
}

type mockClient struct {
	calls       chan string
	lock        sync.RWMutex
	mockEnviron clientEnviron
}

func (m *mockClient) mockCall(call string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.calls <- call
}

func (m *mockClient) ProcessDyingEnviron() error {
	m.mockCall("ProcessDyingEnviron")
	if m.mockEnviron.HasMachinesAndServices {
		return errors.Errorf("found documents for environment with uuid %s: 1 cleanups doc, 1 constraints doc, 1 envusers doc, 1 leases doc, 1 settings doc", m.mockEnviron.UUID)
	}
	m.mockEnviron.Life = state.Dead
	t := time.Now()
	m.mockEnviron.TimeOfDeath = &t
	return nil
}

func (m *mockClient) RemoveEnviron() error {
	m.mockCall("RemoveEnviron")
	m.mockEnviron.Removed = true
	return nil
}

func (m *mockClient) EnvironInfo() (params.ReaperEnvironInfoResult, error) {
	m.mockCall("EnvironInfo")
	result := params.ReaperEnvironInfo{
		Life:        params.Life(m.mockEnviron.Life.String()),
		UUID:        m.mockEnviron.UUID,
		Name:        "dummy",
		GlobalName:  "bob/dummy",
		IsSystem:    m.mockEnviron.IsSystem,
		TimeOfDeath: m.mockEnviron.TimeOfDeath,
	}
	return params.ReaperEnvironInfoResult{Result: result}, nil
}
