package utils

import (
	"strings"
)

// IsUbuntu executes lxb_release to see if the host OS is Ubuntu.
func IsUbuntu() bool {
	out, err := RunCommand("lsb_release", "-i", "-s")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "Ubuntu"
}
