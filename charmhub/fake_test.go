// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

type FakeLogger struct {
}

func (l *FakeLogger) IsTraceEnabled() bool {
	return false
}

func (l *FakeLogger) Errorf(format string, args ...interface{}) {}

func (l *FakeLogger) Debugf(format string, args ...interface{}) {}

func (l *FakeLogger) Tracef(format string, args ...interface{}) {}
