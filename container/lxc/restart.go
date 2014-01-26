package lxc

import (
	"os"
)

//If restart dir exists, return true. Otherwise, return false.
//TODO Should this test LXC version, not existence of restart dir?
func useRestartDir() bool {
	if _, err := os.Stat(LxcRestartDir); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
