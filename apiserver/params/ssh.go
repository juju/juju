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
