// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

type fakeLogger struct {
}

func (l *fakeLogger) IsTraceEnabled() bool {
	return false
}

func (l *fakeLogger) Debugf(format string, args ...interface{}) {}

func (l *fakeLogger) Tracef(format string, args ...interface{}) {}
