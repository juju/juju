package backups_test

import (
	"runtime"
	"testing"

	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *testing.T) {
	// TODO(bogdanteleaga): Fix these tests on windows
	if runtime.GOOS == "windows" {
		t.Skip("bug 1403084: Skipping this on windows for now")
	}
	coretesting.MgoTestPackage(t)
}
