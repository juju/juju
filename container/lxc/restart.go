package lxc

import (
	"os"
)

func restartDirExists() bool {
	if _, err := os.Stat(LxcRestartDir); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
