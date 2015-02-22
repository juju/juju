// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Rsyslog", 0, NewRsyslogAPI)
}

// RsyslogAPI implements the API used by the rsyslog worker.
type RsyslogAPI struct {
	*common.EnvironWatcher

	st             *state.State
	resources      *common.Resources
	authorizer     common.Authorizer
	StateAddresser *common.StateAddresser
	canModify      bool
}

// NewRsyslogAPI creates a new instance of the Rsyslog API.
func NewRsyslogAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*RsyslogAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}
	return &RsyslogAPI{
		EnvironWatcher: common.NewEnvironWatcher(st, resources, authorizer),
		st:             st,
		authorizer:     authorizer,
		resources:      resources,
		canModify:      authorizer.AuthEnvironManager(),
		StateAddresser: common.NewStateAddresser(st),
	}, nil
}

// SetRsyslogCert sets the rsyslog CACert.
func (api *RsyslogAPI) SetRsyslogCert(args params.SetRsyslogCertParams) (params.ErrorResult, error) {
	var result params.ErrorResult
	if !api.canModify {
		result.Error = common.ServerError(common.ErrBadCreds)
		return result, nil
	}
	if _, err := cert.ParseCert(string(args.CACert)); err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}

	attrs := map[string]interface{}{
		"rsyslog-ca-cert": string(args.CACert),
		"rsyslog-ca-key":  string(args.CAKey),
	}
	if err := api.st.UpdateEnvironConfig(attrs, nil, nil); err != nil {
		result.Error = common.ServerError(err)
	}
	return result, nil
}

// GetRsyslogConfig returns a RsyslogConfigResult.
func (api *RsyslogAPI) GetRsyslogConfig(args params.Entities) (params.RsyslogConfigResults, error) {
	result := params.RsyslogConfigResults{
		Results: make([]params.RsyslogConfigResult, len(args.Entities)),
	}
	cfg, err := api.st.EnvironConfig()
	if err != nil {
		return result, err
	}
	for i := range args.Entities {
		rsyslogCfg, err := newRsyslogConfig(cfg, api)
		if err == nil {
			result.Results[i] = params.RsyslogConfigResult{
				CACert:    rsyslogCfg.CACert,
				CAKey:     rsyslogCfg.CAKey,
				Port:      rsyslogCfg.Port,
				HostPorts: params.FromNetworkHostPorts(rsyslogCfg.HostPorts),
			}
		} else {
			result.Results[i].Error = common.ServerError(err)
		}
	}
	return result, nil
}

// WatchForRsyslogChanges starts a watcher to track if there are changes
// that require we update the rsyslog.d configurations for a machine and/or unit.
func (api *RsyslogAPI) WatchForRsyslogChanges(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i := range args.Entities {
		err := common.ErrPerm
		if api.authorizer.AuthMachineAgent() || api.authorizer.AuthUnitAgent() {
			watch := api.st.WatchAPIHostPorts()
			// Consume the initial event. Technically, API
			// calls to Watch 'transmit' the initial event
			// in the Watch response. But NotifyWatchers
			// have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result.Results[i].NotifyWatcherId = api.resources.Register(watch)
				err = nil
			} else {
				err = watcher.EnsureErr(watch)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil

}
