package updateseries

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("updateseries only runs on Linux")
	}
	gc.TestingT(t)
}
