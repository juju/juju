// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"github.com/juju/proxy"
)

// Config contains information from the model config that is needed for
// container cloud-init.
type Config struct {
	EnableOSRefreshUpdate      bool
	EnableOSUpgrade            bool
	ProviderType               string
	SSLHostnameVerification    bool
	LegacyProxy                proxy.Settings
	JujuProxy                  proxy.Settings
	AptProxy                   proxy.Settings
	SnapProxy                  proxy.Settings
	SnapStoreAssertions        string
	SnapStoreProxyID           string
	SnapStoreProxyURL          string
	AptMirror                  string
	CloudInitUserData          map[string]interface{}
	ContainerInheritProperties string
}
