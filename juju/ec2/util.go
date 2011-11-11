package ec2

import (
	"launchpad.net/juju/go/schema"
)

// this stuff could/should be in the schema package.

type checkerFunc func(v interface{}, path []string) (newv interface{}, err error)

func (f checkerFunc) Coerce(v interface{}, path []string) (newv interface{}, err error) {
	return f(v, path)
}

func combineCheckers(checkers ...schema.Checker) schema.Checker {
	f := func(v interface{}, path []string) (newv interface{}, err error) {
		for _, c := range checkers {
			v, err = c.Coerce(v, path)
			if err != nil {
				return nil, err
			}
		}
		return v, nil
	}
	return checkerFunc(f)
}

func oneOf(values ...interface{}) schema.Checker {
	c := make([]schema.Checker, len(values))
	for i, v := range values {
		c[i] = schema.Const(v)
	}
	return schema.OneOf(c...)
}
