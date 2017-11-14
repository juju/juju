// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

// Client allows access to the CAAS provisioner API end point.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASProvisioner")
	return &Client{
		facade: facadeCaller,
	}
}

// ConnectionConfig holds attributes needed to connect to the CAAS cloud.
type ConnectionConfig struct {
	Endpoint       string
	CACertificates []string
	CertData       []byte
	KeyData        []byte
	Username       string
	Password       string
}

// ConnectionConfig returns the config info required for the
// provisioner to connect to the CAAS cloud
func (st *Client) ConnectionConfig() (*ConnectionConfig, error) {
	var result params.CAASConnectionConfig
	if err := st.facade.FacadeCall("ConnectionConfig", nil, &result); err != nil {
		return nil, err
	}
	return &ConnectionConfig{
		Endpoint:       result.Endpoint,
		CACertificates: result.CACertificates,
		CertData:       result.CertData,
		KeyData:        result.KeyData,
		Username:       result.Username,
		Password:       result.Password,
	}, nil
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (st *Client) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := st.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
