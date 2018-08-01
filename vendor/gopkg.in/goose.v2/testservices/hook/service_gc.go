// +build !gccgo

package hook

import (
	"runtime"
	"strings"
)

// currentServiceMethodName returns the method executing on the service when ProcessControlHook was invoked.
func (s *TestService) currentServiceMethodName() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		panic("current method name cannot be found")
	}
	return unqualifiedMethodName(pc)
}

func unqualifiedMethodName(pc uintptr) string {
	f := runtime.FuncForPC(pc)
	fullName := f.Name()
	nameParts := strings.Split(fullName, ".")
	return nameParts[len(nameParts)-1]
}
