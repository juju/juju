// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

var (
	runtimeGOOS = runtime.GOOS

	smallOplogSizeMB   = 512
	regularOplogSizeMB = 1024
	smallOplogBoundary = 15360.0

	availSpace = fsAvailSpace
)

// defaultOplogSize returns the default size in MB for the
// mongo oplog based on the directory of the mongo database.
//
// Since we limit the maximum oplog size to 1GB and every change
// in opLogSize requires mongo restart we are not using the default
// MongoDB formula but simply using 512MB for small disks and 1GB
// for larger ones.
func defaultOplogSize(dir string) (int, error) {
	// "For 64-bit OS X systems, MongoDB allocates 183 megabytes of
	// space to the oplog."
	if runtimeGOOS == "darwin" {
		return 183, nil
	}

	avail, err := availSpace(dir)
	if err != nil {
		return -1, err
	}
	if avail < smallOplogBoundary {
		return smallOplogSizeMB, nil
	} else {
		return regularOplogSizeMB, nil
	}
}

// fsAvailSpace returns the available space in MB on the
// filesystem containing the specified directory.
func fsAvailSpace(dir string) (avail float64, err error) {
	var stderr bytes.Buffer
	cmd := exec.Command("df", dir)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		err := fmt.Errorf("df failed: %v", err)
		if stderr.Len() > 0 {
			err = fmt.Errorf("%s (%q)", err, stderr.String())
		}
		return -1, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		logger.Errorf("unexpected output: %q", out)
		return -1, fmt.Errorf("could not determine available space on %q", dir)
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		logger.Errorf("unexpected output: %q", out)
		return -1, fmt.Errorf("could not determine available space on %q", dir)
	}
	kilobytes, err := strconv.Atoi(fields[3])
	if err != nil {
		return -1, err
	}
	return float64(kilobytes) / 1024, err
}
