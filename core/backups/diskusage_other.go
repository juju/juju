// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !linux

package backups

// Controllers only run on linux. These stubs exist because this package
// is imported by the juju CLI (cmd/juju/backups) and by rpc/params,
// which do build for other platforms. Disk usage cannot be determined
// there, so 0 is reported: CheckSpaceFor then fails rather than
// assume there is enough space.

// diskFree returns 0 on platforms where the free space cannot be
// determined.
func diskFree(string) (uint64, error) {
	return 0, nil
}

// diskTotal returns 0 on platforms where the disk size cannot be
// determined.
func diskTotal(string) (uint64, error) {
	return 0, nil
}
