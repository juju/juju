// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxcutils

import (
	"bufio"
	"os"
	"strings"

	"github.com/juju/errors"
)

func runningInsideLXC() (bool, error) {
	file, err := os.Open(initProcessCgroupFile)
	if err != nil {
		return false, errors.Trace(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) != 3 {
			return false, errors.Errorf("malformed cgroup file")
		}
		if fields[2] != "/" {
			// When running in a container the anchor point will be
			// something other than "/".
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, errors.Annotate(err, "failed to read cgroup file")
	}
	return false, nil
}
