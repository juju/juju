package checkers

import (
	"reflect"
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

// Set sets the value pointed to by the given
// destination to the given value, and returns
// a function to restore it to its original value.
// The value must be assignable to the element
// type of the destination.
func Set(dest, value interface{}) Restorer {
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
