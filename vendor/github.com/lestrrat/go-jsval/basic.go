package jsval

import (
	"errors"
	"reflect"

	"github.com/lestrrat/go-pdebug"
)

func (dv defaultValue) HasDefault() bool {
	return dv.initialized
}

func (dv defaultValue) DefaultValue() interface{} {
	return dv.value
}

func (nc emptyConstraint) Validate(_ interface{}) error {
	return nil
}

func (nc emptyConstraint) HasDefault() bool {
	return false
}

func (nc emptyConstraint) DefaultValue() interface{} {
	return nil
}

func (nc nullConstraint) HasDefault() bool {
	return false
}

func (nc nullConstraint) DefaultValue() interface{} {
	return nil
}

func (nc nullConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("NullConstraint.Validate").BindError(&err)
		defer g.End()
	}

	rv := reflect.ValueOf(v)
	if rv == zeroval {
		return nil
	}

	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		if rv.IsNil() {
			return nil
		}
	}
	return errors.New("value is not null")
}

// Not creates a new NotConstraint. You must pass in the
// child constraint to be run
func Not(c Constraint) *NotConstraint {
	return &NotConstraint{child: c}
}

// HasDefault is a no op for this constraint
func (nc NotConstraint) HasDefault() bool {
	return false
}

// DefaultValue is a no op for this constraint
func (nc NotConstraint) DefaultValue() interface{} {
	return nil
}

// Validate runs the validation, and returns an error unless
// the child constraint fails
func (nc NotConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("NotConstraint.Validate").BindError(&err)
		defer g.End()
	}

	if nc.child == nil {
		return errors.New("'not' constraint does not have a child constraint")
	}

	if err := nc.child.Validate(v); err == nil {
		return errors.New("'not' validation failed")
	}
	return nil
}
