// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// SSHHostKeySet defines SSH host keys for one or more entities
// (typically machines).
type SSHHostKeySet struct {
	EntityKeys []SSHHostKeys `json:"entity-keys"`
}

// SSHHostKeys defines the SSH host keys for one entity.
type SSHHostKeys struct {
	Tag        string   `json:"tag"`
	PublicKeys []string `json:"public-keys"`
}

// SSHProxyResult defines the response from the SSHClient.Proxy API.
type SSHProxyResult struct {
	UseProxy bool `json:"use-proxy"`
}

// SSHTargets is used to specify one or more SSH destinations for
// various APIs on the SSHClient facade.
type SSHTargets struct {
	Targets []SSHTarget `json:"targets"`
}

// SSHTarget specifies a single SSH target (see SSHTargets).
type SSHTarget struct {
	Target string `json:"target"`
}

// SSHAddressResults defines the response from various APIs on the
// SSHClient facade.
type SSHAddressResults struct {
	Results []SSHAddressResult `json:"results"`
}

// SSHAddressResult defines a single SSH address result (see
// SSHAddressResults).
type SSHAddressResult struct {
	Error   *Error `json:"error,omitempty"`
	Address string `json:"address,omitempty"`
}

// SSHPublicKeysResults is used to return SSH public host keys for one
// or more target for the SSHClient.PublicKeys API.
type SSHPublicKeysResults struct {
	Results []SSHPublicKeysResult `json:"results"`
}

// SSHPublicKeysResult is used to return the SSH public host keys for
// one SSH target (see SSHPublicKeysResults).
type SSHPublicKeysResult struct {
	Error      *Error   `json:"error,omitempty"`
	PublicKeys []string `json:"public-keys,omitempty"`
}
