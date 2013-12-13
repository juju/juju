// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build !windows

package ssh

import (
	"fmt"
	"launchpad.net/juju-core/utils"
)

// keyFingerprint returns the fingerprint and comment for the specified key.
// It calls ssh-keygen to do the work as there is no equivalent Go implementation.
func keyFingerprint(key string) (fingerprint, comment string, err error) {
	// Instead of invoking ssh-keygen directly, it has to be called indirectly via a
	// shell command because it doesn't read from stdin properly.
	// See https://bugzilla.mindrot.org/show_bug.cgi?id=1477
	shellCmd := fmt.Sprintf("ssh-keygen -lf /dev/stdin <<<'%s'", key)
	output, err := utils.RunCommand("bash", "-c", shellCmd)
	if err != nil {
		return "", "", fmt.Errorf("generating key fingerprint: %v", err)
	}
	var ignore string
	n, err := fmt.Sscanf(output, "%s %s %s", &ignore, &fingerprint, &comment)
	if n < 3 {
		return "", "", fmt.Errorf("unexpected ssh-keygen output %q: %v", output, err)
	}
	if comment == "/dev/stdin" {
		comment = ""
	}
	return fingerprint, comment, nil
}
