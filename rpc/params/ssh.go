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

// SSHAddressesResults defines the response from AllAddresses on the SSHClient
// API facade.
type SSHAddressesResults struct {
	Results []SSHAddressesResult `json:"results"`
}

// SSHAddressesResult defines a single result with multiple addresses (see
// SSHAddressesResults).
type SSHAddressesResult struct {
	Error     *Error   `json:"error,omitempty"`
	Addresses []string `json:"addresses"`
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

// SSHHostKeyRequestArg provides a hostname to request the hostkey for.
type SSHHostKeyRequestArg struct {
	Hostname string `json:"hostname"`
}

// PublicSSHHostKeyResult returns the host key for the target hostname.
// Additionally, it returns the controller's SSH jump server's host key.
//
// We return the jump server's host key as to SSH to this unit, clients MUST
// jump through the controller.
type PublicSSHHostKeyResult struct {
	Error             *Error `json:"error,omitempty"`
	HostKey           string `json:"host-key"`
	JumpServerHostKey string `json:"jump-server-host-key"`
}
