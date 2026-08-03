// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"fmt"

	"github.com/juju/utils/v4/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// jujuEphemeralCommentPrefix is the comment prefix used to identify ephemeral
// keys in the authorized_keys file so they can be removed when no longer
// needed (for example when the worker is restarted).
const jujuEphemeralCommentPrefix = ssh.JujuCommentPrefix + "Ephemeral:"

// ensureJujuEphemeralComment returns a key with comment in the OpenSSH
// authorized_keys file format. Note the string does not include a new line at
// the end for compatibility with juju/utils `ParseAuthorisedKey`.
// The comment is used to identify the ephemeral keys and delete them when the
// worker is restarted.
func ensureJujuEphemeralComment(key gossh.PublicKey, comment string) string {
	authKey := gossh.MarshalAuthorizedKey(key)
	// Strip off the trailing new line so we can add a comment.
	authKey = authKey[:len(authKey)-1]
	comment = jujuEphemeralCommentPrefix + comment
	return fmt.Sprintf("%s %s", authKey, comment)
}
