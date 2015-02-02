// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"github.com/juju/errors"
)

var (
	// ConfOptionalFields is the names of the Conf fields that are
	// optional. They may be empty and they may not even be supported
	// on any given InitSystem implementation.
	ConfOptionalFields = []string{
		"Env",
		"Limit",
		"Out",
	}
)

// Conf contains all the information an init system may need in order
// to describe a service. It is used both when enabling a service and
// in serialization.
//
// Some fields are optional and may not be supported on all InitSystem
// implementations. In the latter case such a field should not be set.
// If it is then any InitSystem call for that init system involving the
// Conf will fail with an ErrUnsupportedField, wrapped in
// errors.NotSupported. Likewise, for fields with mapped values:
// ErrUnsupportedItem. Either error may be resolved by passing them to
// the Conf.Repair method, which performs an in-place fix if possible.
type Conf struct {
	// Desc is a description of the service.
	Desc string

	// Cmd is the command (with arguments) that will be run. It may be
	// just the path to another scipt that holds a more complex command
	// or a series of them.
	Cmd string

	// Env holds the environment variables that will be set when the
	// command runs. Env is optional and may not be supported by all
	// InitSystem implementations.
	//
	// Not all init systems support all environment variables. If a
	// variable is not supported then the InitSystem method will fail
	// with errors.NotSupported error holding the string "Env name:"
	// followed by the name of the variable. Likewise for values:
	// "Env value:" followed by the variable name.
	Env map[string]string

	// Limit holds the ulimit values that will be set when the command
	// runs. Limit is optional and may not be supported by all
	// InitSystem implementations.
	//
	// Not all init systems support all environment variables. If a
	// variable is not supported then the InitSystem method will fail
	// with errors.NotSupported error holding the string "Env name:"
	// followed by the name of the variable. Likewise for values:
	// "Env value:" followed by the variable name.
	Limit map[string]string

	// Out is the path to the file where the command's output should
	// be written. Out is optional and may not be supported by all
	// InitSystem implementations.
	Out string
}

func (c *Conf) Repair(err error) error {
	if !errors.IsNotSupported(err) {
		// We don't wrap this in errors.Trace since we need to record
		// that the error passed through here.
		return err
	}
	switch rawErr := errors.Cause(err).(type) {
	case ErrUnsupportedField:
		if rawErr.Value {
			return errors.NewNotValid(err, "")
		}

		switch rawErr.Field {
		case "Desc", "Cmd":
			// Oops. This is supposed to be supported.
			return errors.Annotatef(err, `required field %q`, rawErr.Field)
		case "Out":
			c.Out = ""
		default:
			return errors.Trace(err)
		}
	case ErrUnsupportedItem:
		if rawErr.Value {
			return errors.NewNotValid(err, "")
		}

		var items map[string]string
		switch rawErr.Field {
		case "Env":
			items = c.Env
		case "Limit":
			items = c.Limit
		default:
			return errors.Trace(err)
		}
		delete(items, rawErr.Key)
	default:
		// Again, we don't use errors.Trace here.
		return err
	}
	return nil
}

func (c Conf) Validate(name string) error {
	if name == "" {
		return errors.NotValidf("missing name")
	}
	if c.Desc == "" {
		return errors.NotValidf("missing Desc")
	}
	if c.Cmd == "" {
		return errors.NotValidf("missing Cmd")
	}
	return nil
}

func (c Conf) Equals(other Conf) bool {
	if c.Desc != other.Desc {
		return false
	}
	if !compareStrMaps(c.Env, other.Env) {
		return false
	}
	if !compareStrMaps(c.Limit, other.Limit) {
		return false
	}
	if c.Cmd != other.Cmd {
		return false
	}
	if c.Out != other.Out {
		return false
	}
	return true
}

func compareStrMaps(map1, map2 map[string]string) bool {
	if len(map1) != len(map2) {
		return false
	}
	for key, value1 := range map1 {
		value2, ok := map2[key]
		if !ok {
			return false
		}
		if value1 != value2 {
			return false
		}
	}
	return true
}
