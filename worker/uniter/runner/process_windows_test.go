package runner_test

import (
	"os"
)

func processExists(pid int) bool {
	_, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return true
}
