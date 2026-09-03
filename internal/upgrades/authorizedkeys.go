// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/ssh"
)

const (
	authorizedKeysFile         = "authorized_keys"
	jujuEphemeralCommentPrefix = ssh.JujuCommentPrefix + "Ephemeral:"
)

var sshUser = "ubuntu"

// removePersistentJujuAuthorizedKeys removes legacy persistent Juju-managed
// keys from the ubuntu user's authorized_keys file. Only keys whose comment
// starts with the Juju prefix (e.g. "Juju:user@host") are selected for
// deletion; they are removed by comment via ssh.DeleteKeysFromFile.
//
// Keys with an ephemeral comment prefix ("Juju:Ephemeral:") are retained, as
// they belong to active reverse tunnels. Manually added keys without a Juju
// comment are never selected for deletion.
//
// Caveat: deletion is fingerprint-based under the hood. If a manually added
// key has the exact same public key material as a Juju-managed key, the
// utility's rewrite may drop both lines, since it indexes keys by
// fingerprint. Duplicate fingerprints across Juju and manual keys are not
// expected in practice.
func removePersistentJujuAuthorizedKeys(Context) error {
	keys, err := ssh.ListKeys(sshUser, ssh.FullKeys)
	if err != nil {
		return errors.Trace(err)
	}

	comments := make([]string, 0, len(keys))
	for _, key := range keys {
		parsed, err := ssh.ParseAuthorisedKey(key)
		if err != nil {
			return errors.Trace(err)
		}
		if strings.HasPrefix(parsed.Comment, ssh.JujuCommentPrefix) &&
			!strings.HasPrefix(parsed.Comment, jujuEphemeralCommentPrefix) {
			comments = append(comments, parsed.Comment)
		}
	}
	if len(comments) == 0 {
		return nil
	}
	return errors.Trace(ssh.DeleteKeysFromFile(sshUser, authorizedKeysFile, comments))
}
