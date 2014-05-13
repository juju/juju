// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"fmt"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

const rsyslogAPI = "Rsyslog"

// RsyslogConfig
type RsyslogConfig struct {
	*config.Config
}

// State provides access to the Rsyslog API facade.
type State struct {
	*common.EnvironWatcher
	caller base.Caller
}

func (st *State) call(method string, params, result interface{}) error {
	return st.caller.Call("Rsyslog", "", method, params, result)
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
func (st *State) SetRsyslogCert(caCert string) error {
	var result params.ErrorResult
	args := params.SetRsyslogCertParams{
		CACert: []byte(caCert),
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

// WatchForChanges ...
func (st *State) WatchForRsyslogChanges(agentTag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag}},
	}
	err := st.call("WatchForRsyslogChanges", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.caller, result)
	return w, nil
}

func (st *State) GetRsyslogConfig() (*config.Config, error) {
	var rsyslogConfig RsyslogConfig
	cfg, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
