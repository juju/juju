// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// SSH is the ssh key that the instance is using
// To connect to an instance using SSH,
// you must associate it with one or more SSH public keys.
// You must first generate the required SSH key pairs,
// by using a tool such as ssh-keygen, and then upload
// the public keys to Oracle Compute Cloud Service.
// You can add, delete, update, and view SSH public keys
type SSH struct {
	Enabled bool   `json:"enabled"`
	Uri     string `json:"uri"`
	Key     string `json:"key"`
	Name    string `json:"name"`
}

// AllSSH represents all the ssh keys stored in the
// oracle cloud account
type AllSSH struct {
	Result []SSH `json:"result"`
}

type AllSSHNames struct {
	Result []string `json:"result"`
}
