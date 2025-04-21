// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"fmt"

	"github.com/juju/utils/v3/ssh"
	gossh "golang.org/x/crypto/ssh"
)

const jujuEphemeralCommentPrefix = ssh.JujuCommentPrefix + "Ephemeral:"

// ensureJujuEphemeralComment returns a key with comment in the OpenSSH authorized_keys file
// format. Note the string does not include a new line at the end for compatibility with
// juju/utils `ParseAuthorisedKey`.
// The comment is used to identify the ephemeral keys and delete them when the worker is restarted.
func ensureJujuEphemeralComment(key gossh.PublicKey, comment string) string {
	auth_key := gossh.MarshalAuthorizedKey(key)
	// Strip off the trailing new line so we can add a comment.
	auth_key = auth_key[:len(auth_key)-1]
	comment = jujuEphemeralCommentPrefix + comment
	return fmt.Sprintf("%s %s", auth_key, comment)
}

// ensureJujuCommentForKeys applies the Juju comment prefix to all keys.
func ensureJujuCommentForKeys(keys []string) []string {
	for i, key := range keys {
		keys[i] = ssh.EnsureJujuComment(key)
	}
	return keys
}
