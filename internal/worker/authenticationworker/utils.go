// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"fmt"
	"strings"

	"github.com/juju/utils/v3/ssh"
)

const jujuEphemeralCommentPrefix = ssh.JujuCommentPrefix + "Ephemeral:"

// ensureJujuEphemeralComment ensures that the key has a Juju ephemeral comment.
// The comment is used to identify the ephemeral keys and delete them when the worker is restarted.
// This is a copy paste from ssh.EnsureJujuComment, but we need to add the ephemeral prefix
// to the comment if it is not already present.
func ensureJujuEphemeralComment(key string) string {
	ak, err := ssh.ParseAuthorisedKey(key)
	// Just return an invalid key as is. The method ssh.AddKeys will return
	// an error if the key is invalid.
	if err != nil {
		logger.Warningf("invalid Juju ssh key %s: %v", key, err)
		return key
	}
	if ak.Comment == "" {
		return fmt.Sprintf("%s %ssshkey", key, jujuEphemeralCommentPrefix)
	} else {
		// Add the Juju prefix to the comment if necessary.
		if !strings.HasPrefix(ak.Comment, jujuEphemeralCommentPrefix) {
			commentIndex := strings.LastIndex(key, ak.Comment)
			keyWithoutComment := key[:commentIndex]
			return keyWithoutComment + jujuEphemeralCommentPrefix + ak.Comment
		}
	}
	return key
}

// ensureJujuCommentForKeys applies the Juju comment prefix to all keys.
func ensureJujuCommentForKeys(keys []string) []string {
	for i, key := range keys {
		keys[i] = ssh.EnsureJujuComment(key)
	}
	return keys
}
