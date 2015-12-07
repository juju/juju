// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	rsyslog "github.com/juju/syslog"
)

var (
	RestartRsyslog          = &restartRsyslog
	LogDir                  = &logDir
	RsyslogConfDir          = &rsyslogConfDir
	LookupUser              = &lookupUser
	DialSyslog              = &dialSyslog
	NewRsyslogConfigHandler = newRsyslogConfigHandler
)

func SyslogTargets() []*rsyslog.Writer {
	syslogTargetsLock.Lock()
	targets := make([]*rsyslog.Writer, len(syslogTargets))
	copy(targets, syslogTargets)
	syslogTargetsLock.Unlock()
	return targets
}
