// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rsyslog

import (
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// RsyslogAPI implements the API used by the rsyslog worker.
type RsyslogAPI struct {
	*common.EnvironWatcher
	st        *state.State
	canModify bool
}

// NewRsyslogAPI creates a new instance of the Rsyslog API.
func NewRsyslogAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*RsyslogAPI, error) {
	// Can always watch for environ changes.
	getCanWatch := common.AuthAlways(true)
	// Does not get the secrets.
	getCanReadSecrets := common.AuthAlways(false)
	return &RsyslogAPI{
		EnvironWatcher: common.NewEnvironWatcher(st, resources, getCanWatch, getCanReadSecrets),
		st:             st,
		canModify:      authorizer.AuthEnvironManager(),
	}, nil
}

func (api *RsyslogAPI) SetRsyslogCert(args params.SetRsyslogCertParams) (params.ErrorResult, error) {
	var result params.ErrorResult
	if !api.canModify {
		result.Error = common.ServerError(common.ErrBadCreds)
		return result, nil
	}
	if _, err := cert.ParseCert(args.CACert); err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	old, err := api.st.EnvironConfig()
	if err != nil {
		return params.ErrorResult{}, err
	}
	cfg, err := old.Apply(map[string]interface{}{"rsyslog-ca-cert": string(args.CACert)})
	if err != nil {
		result.Error = common.ServerError(err)
	} else {
		if err := api.st.SetEnvironConfig(cfg, old); err != nil {
			result.Error = common.ServerError(err)
		}
	}
	return result, nil
}
