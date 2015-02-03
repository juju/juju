// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems

import (
	"encoding/json"

	"github.com/juju/errors"
)

const (
	errUnknownField  = "reported unknown field %q as unsupported"
	errRequiredField = "reported required field %q as unsupported"
	errNonMapField   = "reported field %q as a map"
)

var (
	// ErrBadInitSystemFailure is used to indicate that an InitSystem
	// implementation is returning errors that it shouldn't be.
	ErrBadInitSystemFailure = errors.New("init system returned an invalid error")

	// ErrUnfixableField is returned from Conf.Repair when a field could
	// not be fixed.
	ErrUnfixableField = errors.New("could not fix conf field")
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
	Desc string `json:"description"`

	// Cmd is the command (with arguments) that will be run. It may be
	// just the path to another scipt that holds a more complex command
	// or a series of them.
	Cmd string `json:"startexec"`

	// Env holds the environment variables that will be set when the
	// command runs. Env is optional and may not be supported by all
	// InitSystem implementations.
	//
	// Not all init systems support all environment variables. If a
	// variable is not supported then the InitSystem method will fail
	// with errors.NotSupported error holding the string "Env name:"
	// followed by the name of the variable. Likewise for values:
	// "Env value:" followed by the variable name.
	Env map[string]string `json:"env,omitempty"`

	// Limit holds the ulimit values that will be set when the command
	// runs. Limit is optional and may not be supported by all
	// InitSystem implementations.
	//
	// Not all init systems support all environment variables. If a
	// variable is not supported then the InitSystem method will fail
	// with errors.NotSupported error holding the string "Env name:"
	// followed by the name of the variable. Likewise for values:
	// "Env value:" followed by the variable name.
	Limit map[string]string `json:"limit,omitempty"`

	// Out is the path to the file where the command's output should
	// be written. Out is optional and may not be supported by all
	// InitSystem implementations.
	Out string `json:"out,omitempty"`
}

// Repair correct the problem reported by the error, if possible. If the
// error is unrecognized then it is returned as-is with no change to the
// conf. If it is recognized but the reported field is required then a
// new error is returned that reports the situation. Likewise if a
// non-map field is used in ErrUnsupportedItem or an unrecognized field
// is in the error. Otherwise if the field cannot be fixed then a new
// error is returned which indicates that.
func (c *Conf) Repair(err error) error {
	var unfixableField string
	switch rawErr := errors.Cause(err).(type) {
	case *ErrUnsupportedField:
		switch rawErr.Field {
		case "Desc", "Cmd":
			// Oops. The field is *supposed* to be supported.
			return errors.Wrapf(err, ErrBadInitSystemFailure, errRequiredField, rawErr.Field)
		case "Out":
			c.Out = ""
		case "Env":
			c.Env = nil
		case "Limit":
			c.Limit = nil
		default:
			return errors.Wrapf(err, ErrBadInitSystemFailure, errUnknownField, rawErr.Field)
		}
	case *ErrUnsupportedItem:
		var items map[string]string
		switch rawErr.Field {
		case "Env":
			items = c.Env
		case "Limit":
			items = c.Limit
		case "Desc", "Cmd", "Out":
			// Oops. These fields should not have been used here.
			return errors.Wrapf(err, ErrBadInitSystemFailure, errNonMapField, rawErr.Field)
		default:
			return errors.Wrapf(err, ErrBadInitSystemFailure, errUnknownField, rawErr.Field)
		}
		// Remove the item.
		delete(items, rawErr.Key)
	default:
		// We don't wrap this in errors.Trace since we need to record
		// that the error passed through here.
		return err
	}

	if unfixableField != "" {
		return errors.Wrapf(err, ErrUnfixableField, unfixableField)
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

// SerializeJSON converts the conf into a JSON string.
func SerializeJSON(conf Conf) ([]byte, error) {
	data, err := json.MarshalIndent(&conf, "", " ")
	return data, errors.Trace(err)
}

// DeserializeJSON converts the data into the equivalent Conf, if possible.
func DeserializeJSON(data []byte) (Conf, error) {
	var conf Conf
	err := json.Unmarshal(data, &conf)
	return conf, errors.Trace(err)
}
