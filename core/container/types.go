// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/proxy"

	"github.com/juju/juju/rpc/params"
)

// TODO: may not need this
type Config struct {
	UpdateBehavior             *params.UpdateBehavior
	ProviderType               string
	AuthorizedKeys             string
	SSLHostnameVerification    bool
	LegacyProxy                proxy.Settings
	JujuProxy                  proxy.Settings
	AptProxy                   proxy.Settings
	AptMirror                  string
	SnapProxy                  proxy.Settings
	SnapStoreAssertions        string
	SnapStoreProxyID           string
	SnapStoreProxyURL          string
	CloudInitUserData          map[string]any
	ContainerInheritProperties string
}
