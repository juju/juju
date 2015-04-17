// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"fmt"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
)

const rsyslogAPI = "Rsyslog"

// RsyslogConfig holds the values needed for the rsyslog worker
type RsyslogConfig struct {
	CACert string
	CAKey  string
	// Port is only used by state servers as the port to listen on.
	Port      int
	HostPorts []network.HostPort
}

// State provides access to the Rsyslog API facade.
type State struct {
	facade base.FacadeCaller
}

// NewState creates a new client-side Rsyslog facade.
func NewState(caller base.APICaller) *State {
	return &State{facade: base.NewFacadeCaller(caller, rsyslogAPI)}
}

// SetRsyslogCert sets the rsyslog CA and Key certificates.
// The CA cert is used to verify the server's identify and establish
// a TLS session. The Key is used to allow us to properly regenerate
// rsyslog server certificates when adding and removing
// state servers with ensure-availability.
func (st *State) SetRsyslogCert(caCert, caKey string) error {
	var result params.ErrorResult
	args := params.SetRsyslogCertParams{
		CACert: []byte(caCert),
		CAKey:  []byte(caKey),
	}
	err := st.facade.FacadeCall("SetRsyslogCert", args, &result)
	if err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// WatchForRsyslogChanges returns a new NotifyWatcher.
func (st *State) WatchForRsyslogChanges(agentTag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag}},
	}

	err := st.facade.FacadeCall("WatchForRsyslogChanges", args, &results)
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
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// GetRsyslogConfig returns a RsyslogConfig.
func (st *State) GetRsyslogConfig(agentTag string) (*RsyslogConfig, error) {
	var results params.RsyslogConfigResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag}},
	}
	err := st.facade.FacadeCall("GetRsyslogConfig", args, &results)
	if err != nil {
		return nil, err
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	return &RsyslogConfig{
		CACert:    result.CACert,
		CAKey:     result.CAKey,
		Port:      result.Port,
		HostPorts: params.NetworkHostPorts(result.HostPorts),
	}, nil
}
