// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import "github.com/juju/loggo"

type FakeLogger struct {
}

func (l *FakeLogger) IsTraceEnabled() bool {
	return false
}

func (l *FakeLogger) Errorf(format string, args ...interface{}) {}

func (l *FakeLogger) Tracef(format string, args ...interface{}) {}

func (l *FakeLogger) ChildWithLabels(string, ...string) loggo.Logger {
	return loggo.Logger{}
}
