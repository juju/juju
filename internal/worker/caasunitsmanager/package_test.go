// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitsmanager

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/hub_mock.go github.com/juju/juju/internal/worker/caasunitsmanager Hub

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type Manager interface {
	StopUnitRequest(string, interface{})
	StartUnitRequest(string, interface{})
	UnitStatusRequest(string, interface{})
}

func (w *manager) StopUnitRequest(topic string, data interface{}) {
	w.stopUnitRequest(topic, data)
}

func (w *manager) StartUnitRequest(topic string, data interface{}) {
	w.startUnitRequest(topic, data)
}

func (w *manager) UnitStatusRequest(topic string, data interface{}) {
	w.unitStatusRequest(topic, data)
}
