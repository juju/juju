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

// SSHConnRequestArg holds a tunnel ID identifying a one-shot SSH connection
// request that the sshsession worker wants to read.
type SSHConnRequestArg struct {
	// TunnelID uniquely identifies the SSH connection request.
	TunnelID string `json:"tunnel-id"`
}

// SSHConnRequestResult holds the details of a one-shot SSH connection request
// returned to the machine agent's sshsession worker.
type SSHConnRequestResult struct {
	Error *Error `json:"error,omitempty"`
	// MachineName is the name of the machine the request targets.
	MachineName string `json:"machine-name"`
	// ControllerAddresses are the controller addresses the machine agent may
	// reverse-dial to establish the tunnel.
	ControllerAddresses []string `json:"controller-addresses"`
	// Username is the reverse-tunnel username to authenticate with.
	Username string `json:"username"`
	// Password is the reverse-tunnel JWT credential.
	Password string `json:"password"`
	// UnitPort is the local port to forward the tunnel to (0 means determine
	// dynamically).
	UnitPort int `json:"unit-port"`
	// EphemeralPublicKey is the ephemeral public key to authorise for the
	// lifetime of the tunnel.
	EphemeralPublicKey []byte `json:"ephemeral-public-key"`
}

// SSHControllerSSHPortResult holds the port the controller SSH jump server
// listens on.
type SSHControllerSSHPortResult struct {
	Error *Error `json:"error,omitempty"`
	// Port is the controller SSH jump server port.
	Port int `json:"port"`
}

// SSHControllerPublicKeyResult holds the marshalled controller SSH jump server
// host public key, used by the machine agent to pin the host key when
// reverse-dialling.
type SSHControllerPublicKeyResult struct {
	Error *Error `json:"error,omitempty"`
	// PublicKey is the marshalled controller SSH jump server host public key.
	PublicKey []byte `json:"public-key"`
}
