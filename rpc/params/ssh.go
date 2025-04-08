// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/network"
)

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

// PublicSSHHostKeyResult returns the public key for the target hostname
// in SSH wire format.
// Additionally, it returns the controller's jump server's public key
// in SSH wire format.
type PublicSSHHostKeyResult struct {
	Error               *Error `json:"error,omitempty"`
	PublicKey           []byte `json:"public-key"`
	JumpServerPublicKey []byte `json:"jump-server-public-key"`
}

// SSHConnRequestArg holds the necessary info to create a ssh connection requests.
type SSHConnRequestArg struct {
	TunnelID            string                 `json:"tunnel-id"`
	ModelUUID           string                 `json:"model-uuid"`
	MachineId           string                 `json:"machine-id"`
	Expires             time.Time              `json:"expires"`
	Username            string                 `json:"username"`
	Password            string                 `json:"password"`
	ControllerAddresses network.SpaceAddresses `json:"controller-addresses"`
	UnitPort            int                    `json:"unit-port"`
	EphemeralPublicKey  []byte                 `json:"ephemeral-public-key"`
}

// SSHConnRequestRemoveArg holds the necessary info to remove a ssh connection requests.
type SSHConnRequestRemoveArg struct {
	TunnelID  string `json:"tunnel-id"`
	ModelUUID string `json:"model-uuid"`
	MachineId string `json:"machine-id"`
}

// SSHConnRequest holds the fields returned when you get a SSH connection request.
type SSHConnRequest struct {
	Expires             time.Time              `json:"expires"`
	Username            string                 `json:"username"`
	Password            string                 `json:"password"`
	ControllerAddresses network.SpaceAddresses `json:"addresses"`
	UnitPort            int                    `json:"unit-port"`
	EphemeralPublicKey  []byte                 `json:"ephemeral-public-key"`
}

// SSHConnRequestResult holds the result of a SSH connection request.
type SSHConnRequestResult struct {
	Error          *Error         `json:"error,omitempty"`
	SSHConnRequest SSHConnRequest `json:"ssh-conn-request"`
}

// SSHHostKeyResult holds the private host key.
type SSHHostKeyResult struct {
	Error   *Error `json:"error,omitempty"`
	HostKey []byte `json:"host-key"`
}

// VerifyPublicKeyArgs is used to verify the Public Key presented is
// inside of the model's config.
type ListAuthorizedKeysArgs struct {
	ModelUUID string `json:"model_uuid"`
}

// ListAuthorizedKeysResult is used to return the public keys for a model.
type ListAuthorizedKeysResult struct {
	Error          *Error   `json:"error,omitempty"`
	AuthorizedKeys []string `json:"public-keys,omitempty"`
}
