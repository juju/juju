// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"fmt"

	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/utils/v3/ssh"
)

const jujuEphemeralCommentPrefix = ssh.JujuCommentPrefix + "Ephemeral:"

// ensureJujuEphemeralComment returns a key for inclusion in an OpenSSH authorized_keys file
// that includes a comment to identify the key.
// The comment is used to identify the ephemeral keys and delete them when the worker is restarted.
func ensureJujuEphemeralComment(key gossh.PublicKey) string {
	auth_key := gossh.MarshalAuthorizedKey(key)
	// Strip off the trailing new line so we can add a comment.
	auth_key = auth_key[:len(auth_key)-1]
	return fmt.Sprintf("%s %s", auth_key, jujuEphemeralCommentPrefix)
}

// ensureJujuCommentForKeys applies the Juju comment prefix to all keys.
func ensureJujuCommentForKeys(keys []string) []string {
	for i, key := range keys {
		keys[i] = ssh.EnsureJujuComment(key)
	}
	return keys
}
