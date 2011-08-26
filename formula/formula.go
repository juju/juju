package formula

import (
	"fmt"
	"launchpad.net/ensemble/go/schema"
	"os"
	"strconv"
	"strings"
)

func errorf(format string, args ...interface{}) os.Error {
	return os.NewError(fmt.Sprintf(format, args...))
}

// ParseId splits a formula identifier into its constituting parts.
func ParseId(id string) (namespace string, name string, rev int, err os.Error) {
	colon := strings.Index(id, ":")
	if colon == -1 {
		err = errorf("Missing formula namespace: %q", id)
		return
	}
	dash := strings.LastIndex(id, "-")
	if dash != -1 {
		rev, err = strconv.Atoi(id[dash+1:])
	}
	if dash == -1 || err != nil {
		err = errorf("Missing formula revision: %q", id)
		return
	}
	namespace = id[:colon]
	name = id[colon+1 : dash]
	return
}

var ifaceSchema = schema.FieldMap(schema.Fields{
	"interface": schema.String(),
	"limit":     schema.OneOf(schema.Const(nil), schema.Int()),
	"optional":  schema.Bool(),
}, nil)

// Schema coercer that expands the interface shorthand notation.
// A consistent format is easier to work with than considering the
// potential difference everywhere.
//
// Supports the following variants::
//
//   provides:
//     server: riak
//     admin: http
//     foobar:
//       interface: blah
//
//   provides:
//     server:
//       interface: mysql
//       limit:
//       optional: false
//
// In all input cases, the output is the fully specified interface
// representation as seen in the mysql interface description above.
func ifaceExpander(limit interface{}) schema.Checker {
	return ifaceExpC{limit}
}

type ifaceExpC struct {
	limit interface{}
}

var (
	stringC = schema.String()
	mapC    = schema.Map(schema.String(), schema.Any())
)

func (c ifaceExpC) Coerce(v interface{}, path []string) (newv interface{}, err os.Error) {
	s, err := stringC.Coerce(v, path)
	if err == nil {
		newv = schema.M{
			"interface": s,
			"limit":     c.limit,
			"optional":  false,
		}
		return
	}

	// Optional values are context-sensitive and/or have
	// defaults, which is different than what KeyDict can
	// readily support. So just do it here first, then
	// coerce to the real schema.
	v, err = mapC.Coerce(v, path)
	if err != nil {
		return
	}
	m := v.(schema.M)
	if _, ok := m["limit"]; !ok {
		m["limit"] = c.limit
	}
	if _, ok := m["optional"]; !ok {
		m["optional"] = false
	}
	return ifaceSchema.Coerce(m, path)
}
