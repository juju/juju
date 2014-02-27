// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
)

const rsyslogAPI = "Rsyslog"

// State provides access to the Rsyslog API facade.
type State struct {
	*common.EnvironWatcher
	caller base.Caller
}

// NewState creates a new client-side Rsyslog facade.
func NewState(caller base.Caller) *State {
	return &State{
		EnvironWatcher: common.NewEnvironWatcher(rsyslogAPI, caller),
		caller:         caller,
	}
}

// SetRsyslogCert sets the rsyslog CA certificate,
// which is used by clients to verify the server's
// identity and establish a TLS session.
func (st *State) SetRsyslogCert(caCert []byte) error {
	var result params.ErrorResult
	args := params.SetRsyslogCertParams{
		CACert: caCert,
	}
	err := st.caller.Call(rsyslogAPI, "", "SetRsyslogCert", args, &result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
