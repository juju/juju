// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	stdcontext "context"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/provider/kubernetes/constants"
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
// The step only applies to IAAS machines: the agent config records the
// provider type, and CAAS agents are skipped explicitly. The ubuntu user
// lookup is kept as a defensive no-op for hosts without the user.
//
// Caveat: deletion is fingerprint-based under the hood. If a manually added
// key has the exact same public key material as a Juju-managed key, the
// utility's rewrite may drop both lines, since it indexes keys by
// fingerprint. Duplicate fingerprints across Juju and manual keys are not
// expected in practice.
func removePersistentJujuAuthorizedKeys(ctx Context) error {
	if ctx.AgentConfig().Value(agent.ProviderType) == constants.CAASProviderType {
		// CAAS pods have no ubuntu user and no cloud-init injected
		// authorized_keys; nothing to clean up.
		return nil
	}

	keys, err := ssh.ListKeysFromFile(sshUser, authorizedKeysFile, ssh.FullKeys)
	if err != nil {
		if errors.Is(err, errors.UserNotFound) {
			// The ubuntu user does not exist on this host; there is
			// nothing to clean up.
			return nil
		}
		return errors.Trace(err)
	}

	comments := make([]string, 0, len(keys))
	for _, key := range keys {
		parsed, err := ssh.ParseAuthorisedKey(key)
		if err != nil {
			logger.Warningf(stdcontext.Background(), "ignoring invalid ssh key %q: %v", key, err)
			continue
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
