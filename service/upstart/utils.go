// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upstart

import (
	"bytes"
	"os/exec"

	"github.com/juju/errors"
)

func runCommand(args ...string) error {
	out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
	if err == nil {
		return nil
	}

	out = bytes.TrimSpace(out)
	if len(out) > 0 {
		return errors.Annotatef(err, "exec %q: (%s)", args, out)
	}
	return errors.Annotatef(err, "exec %q", args)
}
