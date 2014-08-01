// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/juju/juju/juju/arch"
)

const (
	// preallocAlign must divide all preallocated files' sizes.
	preallocAlign = 4096
)

var (
	runtimeGOOS  = runtime.GOOS
	hostWordSize = arch.Info[arch.HostArch()].WordSize

	// zeroes is used by preallocFile to write zeroes to
	// preallocated Mongo data files.
	zeroes = make([]byte, 64*1024)

	minOplogSizeMB = 512
	maxOplogSizeMB = 1024

	availSpace   = fsAvailSpace
	preallocFile = doPreallocFile
)

// preallocOplog preallocates the Mongo oplog in the
// specified Mongo datadabase directory.
func preallocOplog(dir string, oplogSizeMB int) error {
	// preallocFiles expects sizes in bytes.
	sizes := preallocFileSizes(oplogSizeMB * 1024 * 1024)
	prefix := filepath.Join(dir, "local.")
	return preallocFiles(prefix, sizes...)
}

// defaultOplogSize returns the default size in MB for the
// mongo oplog based on the directory of the mongo database.
//
// The size of the oplog is calculated according to the
// formula used by Mongo:
//     http://docs.mongodb.org/manual/core/replica-set-oplog/
//
// NOTE: we deviate from the specified minimum and maximum
//       sizes. Mongo suggests a minimum of 1GB and maximum
//       of 50GB; we set these to 512MB and 1GB respectively.
func defaultOplogSize(dir string) (int, error) {
	if hostWordSize == 32 {
		// "For 32-bit systems, MongoDB allocates about 48 megabytes
		// of space to the oplog."
		return 48, nil
	}

	// "For 64-bit OS X systems, MongoDB allocates 183 megabytes of
	// space to the oplog."
	if runtimeGOOS == "darwin" {
		return 183, nil
	}

	// FIXME calculate disk size on Windows like on Linux below.
	if runtimeGOOS == "windows" {
		return minOplogSizeMB, nil
	}

	avail, err := availSpace(dir)
	if err != nil {
		return -1, err
	}
	size := int(avail * 0.05)
	if size < minOplogSizeMB {
		size = minOplogSizeMB
	} else if size > maxOplogSizeMB {
		size = maxOplogSizeMB
	}
	return size, nil
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

// preallocFiles preallocates n data files, zeroed to make
// up the specified sizes in bytes. The file sizes must be
// multiples of 4096 bytes.
//
// The filenames are constructed by appending the file index
// to the specified prefix.
func preallocFiles(prefix string, sizes ...int) error {
	var err error
	var createdFiles []string
	for i, size := range sizes {
		var created bool
		filename := fmt.Sprintf("%s%d", prefix, i)
		created, err = preallocFile(filename, size)
		if created {
			createdFiles = append(createdFiles, filename)
		}
		if err != nil {
			break
		}
	}
	if err != nil {
		logger.Debugf("cleaning up after preallocation failure: %v", err)
		for _, filename := range createdFiles {
			if err := os.Remove(filename); err != nil {
				logger.Errorf("failed to remove %q: %v", filename, err)
			}
		}
	}
	return err
}

// preallocFileSizes returns a slice of file sizes
// that make up the specified total size, exceeding
// the specified total as necessary to pad the
// remainder to a multiple of 4096 bytes.
func preallocFileSizes(totalSize int) []int {
	// Divide the total size into 512MB chunks, and
	// then round up the remaining chunk to a multiple
	// of 4096 bytes.
	const maxChunkSize = 512 * 1024 * 1024
	var sizes []int
	remainder := totalSize % maxChunkSize
	if remainder > 0 {
		aligned := remainder + preallocAlign - 1
		aligned = aligned - (aligned % preallocAlign)
		sizes = []int{aligned}
	}
	for i := 0; i < totalSize/maxChunkSize; i++ {
		sizes = append(sizes, maxChunkSize)
	}
	return sizes
}

// doPreallocFile creates a file and writes zeroes up to the specified
// extent. If the file exists already, nothing is done and no error
// is returned.
func doPreallocFile(filename string, size int) (created bool, err error) {
	if size%preallocAlign != 0 {
		return false, fmt.Errorf("specified size %v for file %q is not a multiple of %d", size, filename, preallocAlign)
	}
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if os.IsExist(err) {
		// already exists, don't overwrite
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to open mongo prealloc file %q: %v", filename, err)
	}
	defer f.Close()
	for written := 0; written < size; {
		n := len(zeroes)
		if n > (size - written) {
			n = size - written
		}
		n, err := f.Write(zeroes[:n])
		if err != nil {
			return true, fmt.Errorf("failed to write to mongo prealloc file %q: %v", filename, err)
		}
		written += n
	}
	return true, nil
}
