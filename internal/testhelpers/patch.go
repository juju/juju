// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package testing

import (
	"os"
	"reflect"
	"strings"
)

// Restorer holds a function that can be used
// to restore some previous state.
type Restorer func()

// Add returns a Restorer that restores first f1
// and then f. It is valid to call this on a nil
// Restorer.
func (f Restorer) Add(f1 Restorer) Restorer {
	return func() {
		f1.Restore()
		if f != nil {
			f.Restore()
		}
	}
}

// Restore restores some previous state.
func (r Restorer) Restore() {
	r()
}

// PatchValue sets the value pointed to by the given destination to the given
// value, and returns a function to restore it to its original value.  The
// value must be assignable to the element type of the destination.
func PatchValue(dest, value interface{}) Restorer {
	destv := reflect.ValueOf(dest).Elem()
	oldv := reflect.New(destv.Type()).Elem()
	oldv.Set(destv)
	valuev := reflect.ValueOf(value)
	if !valuev.IsValid() {
		// This isn't quite right when the destination type is not
		// nilable, but it's better than the complex alternative.
		valuev = reflect.Zero(destv.Type())
	}
	destv.Set(valuev)
	return func() {
		destv.Set(oldv)
	}
}

// PatchEnvironment provides a test a simple way to override a single
// environment variable. A function is returned that will return the
// environment to what it was before.
func PatchEnvironment(name, value string) Restorer {
	oldValue, oldValueSet := os.LookupEnv(name)
	_ = os.Setenv(name, value)
	return func() {
		if oldValueSet {
			_ = os.Setenv(name, oldValue)
		} else {
			_ = os.Unsetenv(name)
		}
	}
}

// PatchEnvPathPrepend provides a simple way to prepend path to the start of the
// PATH environment variable. Returns a function that restores the environment
// to what it was before.
func PatchEnvPathPrepend(dir string) Restorer {
	return PatchEnvironment("PATH", joinPathLists(dir, os.Getenv("PATH")))
}

func joinPathLists(paths ...string) string {
	return strings.Join(paths, string(os.PathListSeparator))
}
