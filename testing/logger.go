// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

type NoopLogger struct{}

func (NoopLogger) Criticalf(string, ...interface{}) {}
func (NoopLogger) Errorf(string, ...interface{})    {}
func (NoopLogger) Warningf(string, ...interface{})  {}
func (NoopLogger) Infof(string, ...interface{})     {}
func (NoopLogger) Debugf(string, ...interface{})    {}
func (NoopLogger) Tracef(string, ...interface{})    {}

func (NoopLogger) IsErrorEnabled() bool   { return false }
func (NoopLogger) IsWarningEnabled() bool { return false }
func (NoopLogger) IsInfoEnabled() bool    { return false }
func (NoopLogger) IsDebugEnabled() bool   { return false }
func (NoopLogger) IsTraceEnabled() bool   { return false }
