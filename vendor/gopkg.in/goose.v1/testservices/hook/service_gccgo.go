// +build gccgo

package hook

import (
	"runtime"
	"strings"
)

// callerDepth defines the number of stack frames to skip during
// currentServiceMethodName. This value differs for various gccgo
// versions.
var callerDepth int

// namePartsPos defines the position within the raw method name, deliniated by periods.
var namePartsPos = -1 // will panic if we cannot determine the position.

type inner struct{}

func (i *inner) m() {
	for callerDepth = 1; ; callerDepth++ {
		pc, _, _, ok := runtime.Caller(callerDepth)
		if !ok {
			panic("current method name cannot be found")
		}
		if name := runtime.FuncForPC(pc).Name(); name == "hook.setCallerDepth" {
			for i, s := range strings.Split(name, ".") {
				if s == "setCallerDepth" {
					namePartsPos = i
					break
				}
			}
			return
		}
	}
}

type outer struct {
	inner
}

func setCallerDepth0() {
	var o outer
	o.m()
}

func setCallerDepth() {
	setCallerDepth0()
}

func init() {
	setCallerDepth()
	println(callerDepth)
}

// currentServiceMethodName returns the method executing on the service when ProcessControlHook was invoked.
func (s *TestService) currentServiceMethodName() string {
	// We have to go deeper into the stack with gccgo because in a situation like:
	// type Inner { }
	// func (i *Inner) meth {}
	// type Outer { Inner }
	// o = &Outer{}
	// o.meth()
	// gccgo generates a method called "meth" on *Outer, and this shows up
	// on the stack as seen by runtime.Caller (this might be a gccgo bug).
	pc, _, _, ok := runtime.Caller(callerDepth)
	if !ok {
		panic("current method name cannot be found")
	}
	return unqualifiedMethodName(pc)
}

func unqualifiedMethodName(pc uintptr) string {
	f := runtime.FuncForPC(pc)
	fullName := f.Name()
	// This is very fragile.  fullName will be something like:
	// launchpad.net_goose_testservices_novaservice.removeServer.pN49_launchpad.net_goose_testservices_novaservice.Nova
	// so if the number of dots in the full package path changes,
	// We try to figure sniff this value at the top, but it may not work.
	nameParts := strings.Split(fullName, ".")
	return nameParts[namePartsPos]
}
