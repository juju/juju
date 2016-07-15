package checkers

import (
	"strings"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon.v1"
)

// DeclaredCaveat returns a "declared" caveat asserting that the given key is
// set to the given value. If a macaroon has exactly one first party
// caveat asserting the value of a particular key, then InferDeclared
// will be able to infer the value, and then DeclaredChecker will allow
// the declared value if it has the value specified here.
//
// If the key is empty or contains a space, DeclaredCaveat
// will return an error caveat.
func DeclaredCaveat(key string, value string) Caveat {
	if strings.Contains(key, " ") || key == "" {
		return ErrorCaveatf("invalid caveat 'declared' key %q", key)
	}
	return firstParty(CondDeclared, key+" "+value)
}

// NeedDeclaredCaveat returns a third party caveat that
// wraps the provided third party caveat and requires
// that the third party must add "declared" caveats for
// all the named keys.
func NeedDeclaredCaveat(cav Caveat, keys ...string) Caveat {
	if cav.Location == "" {
		return ErrorCaveatf("need-declared caveat is not third-party")
	}
	return Caveat{
		Location:  cav.Location,
		Condition: CondNeedDeclared + " " + strings.Join(keys, ",") + " " + cav.Condition,
	}
}

// Declared implements a checker that will
// check that any "declared" caveats have a matching
// key for their value in the map.
type Declared map[string]string

// Condition implements Checker.Condition.
func (c Declared) Condition() string {
	return CondDeclared
}

// Check implements Checker.Check by checking that the given
// argument holds a key in the map with a matching value.
func (c Declared) Check(_, arg string) error {
	// Note that we don't need to check the condition argument
	// here because it has been specified explicitly in the
	// return from the Condition method.
	parts := strings.SplitN(arg, " ", 2)
	if len(parts) != 2 {
		return errgo.Newf("declared caveat has no value")
	}
	val, ok := c[parts[0]]
	if !ok {
		return errgo.Newf("got %s=null, expected %q", parts[0], parts[1])
	}
	if val != parts[1] {
		return errgo.Newf("got %s=%q, expected %q", parts[0], val, parts[1])
	}
	return nil
}

// InferDeclared retrieves any declared information from
// the given macaroons and returns it as a key-value map.
//
// Information is declared with a first party caveat as created
// by DeclaredCaveat.
//
// If there are two caveats that declare the same key with
// different values, the information is omitted from the map.
// When the caveats are later checked, this will cause the
// check to fail.
func InferDeclared(ms macaroon.Slice) Declared {
	var conflicts []string
	info := make(Declared)
	for _, m := range ms {
		for _, cav := range m.Caveats() {
			if cav.Location != "" {
				continue
			}
			name, rest, err := ParseCaveat(cav.Id)
			if err != nil {
				continue
			}
			if name != CondDeclared {
				continue
			}
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) != 2 {
				continue
			}
			key, val := parts[0], parts[1]
			if oldVal, ok := info[key]; ok && oldVal != val {
				conflicts = append(conflicts, key)
				continue
			}
			info[key] = val
		}
	}
	for _, key := range conflicts {
		delete(info, key)
	}
	return info
}
