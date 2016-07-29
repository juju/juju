// The checkers package provides some standard first-party
// caveat checkers and some primitives for combining them.
package checkers

import (
	"fmt"
	"net"
	"strings"

	"gopkg.in/errgo.v1"
)

// Constants for all the standard caveat conditions.
// First and third party caveat conditions are both defined here,
// even though notionally they exist in separate name spaces.
const (
	CondDeclared     = "declared"
	CondTimeBefore   = "time-before"
	CondClientIPAddr = "client-ip-addr"
	CondClientOrigin = "origin"
	CondError        = "error"
	CondNeedDeclared = "need-declared"
	CondAllow        = "allow"
	CondDeny         = "deny"
)

// ErrCaveatNotRecognized is the cause of errors returned
// from caveat checkers when the caveat was not
// recognized.
var ErrCaveatNotRecognized = errgo.New("caveat not recognized")

// Caveat represents a condition that must be true for a check to
// complete successfully. If Location is non-empty, the caveat must be
// discharged by a third party at the given location.
// This differs from macaroon.Caveat in that the condition
// is not encrypted.
type Caveat struct {
	Location  string
	Condition string
}

// Checker is implemented by types that can check caveats.
type Checker interface {
	// Condition returns the identifier of the condition
	// to be checked - the Check method will be used
	// to check caveats with this identifier.
	//
	// It may return an empty string, in which case
	// it will be used to check any condition
	Condition() string

	// Check checks that the given caveat holds true.
	// The condition and arg are as returned
	// from ParseCaveat.
	//
	// For a checker with an empty condition, a
	// return of bakery.ErrCaveatNotRecognised from
	// this method indicates that the condition was
	// not recognized.
	Check(cond, arg string) error
}

// New returns a new MultiChecker that uses all the
// provided Checkers to check caveats. If several checkers return the
// same condition identifier, all of them will be used.
//
// The cause of any error returned by a checker will be preserved.
//
// Note that because the returned checker implements Checker
// as well as bakery.FirstPartyChecker, calls to New can be nested.
// For example, a checker can be easily added to an existing
// MultiChecker, by doing:
//
//	checker := checkers.New(old, another)
func New(checkers ...Checker) *MultiChecker {
	return &MultiChecker{
		checkers: checkers,
	}
}

// MultiChecker implements bakery.FirstPartyChecker
// and Checker for a collection of checkers.
type MultiChecker struct {
	// TODO it may be faster to initialize a map, but we'd
	// be paying the price of creating and initializing
	// the map vs a few linear scans through a probably-small
	// slice. Let's wait for some real-world numbers.
	checkers []Checker
}

var errBadCaveat = errgo.Newf("bad caveat")

// Check implements Checker.Check.
func (c *MultiChecker) Check(cond, arg string) error {
	// Always check for the error caveat so that we're
	// sure to get a nice error message even when there
	// are no other checkers. This also prevents someone
	// from inadvertently overriding the error condition.
	if cond == CondError {
		return errBadCaveat
	}
	checked := false
	for _, c := range c.checkers {
		checkerCond := c.Condition()
		if checkerCond != "" && checkerCond != cond {
			continue
		}
		if err := c.Check(cond, arg); err != nil {
			if checkerCond == "" && errgo.Cause(err) == ErrCaveatNotRecognized {
				continue
			}
			return errgo.Mask(err, errgo.Any)
		}
		checked = true
	}
	if !checked {
		return ErrCaveatNotRecognized
	}
	return nil
}

// Condition implements Checker.Condition.
func (c *MultiChecker) Condition() string {
	return ""
}

// CheckFirstPartyCaveat implements bakery.FirstPartyChecker.CheckFirstPartyCaveat.
func (c *MultiChecker) CheckFirstPartyCaveat(cav string) error {
	cond, arg, err := ParseCaveat(cav)
	if err != nil {
		// If we can't parse it, perhaps it's in some other format,
		// return a not-recognised error.
		return errgo.WithCausef(err, ErrCaveatNotRecognized, "cannot parse caveat %q", cav)
	}
	if err := c.Check(cond, arg); err != nil {
		return errgo.NoteMask(err, fmt.Sprintf("caveat %q not satisfied", cav), errgo.Any)
	}
	return nil
}

// TODO add multiChecker.CheckThirdPartyCaveat ?
// i.e. make this stuff reusable for 3rd party caveats too.

func firstParty(cond, arg string) Caveat {
	return Caveat{
		Condition: cond + " " + arg,
	}
}

// CheckerFunc implements Checker for a function.
type CheckerFunc struct {
	// Condition_ holds the condition that the checker
	// implements.
	Condition_ string

	// Check_ holds the function to call to make the check.
	Check_ func(cond, arg string) error
}

// Condition implements Checker.Condition.
func (f CheckerFunc) Condition() string {
	return f.Condition_
}

// Check implements Checker.Check
func (f CheckerFunc) Check(cond, arg string) error {
	return f.Check_(cond, arg)
}

// Map is a checker where the various checkers
// are specified as entries in a map, one for each
// condition.
// The cond argument passed to the function
// is always the same as its corresponding key
// in the map.
type Map map[string]func(cond string, arg string) error

// Condition implements Checker.Condition.
func (m Map) Condition() string {
	return ""
}

// Check implements Checker.Check
func (m Map) Check(cond, arg string) error {
	f, ok := m[cond]
	if !ok {
		return ErrCaveatNotRecognized
	}
	if err := f(cond, arg); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return nil
}

// ParseCaveat parses a caveat into an identifier, identifying the
// checker that should be used, and the argument to the checker (the
// rest of the string).
//
// The identifier is taken from all the characters before the first
// space character.
func ParseCaveat(cav string) (cond, arg string, err error) {
	if cav == "" {
		return "", "", fmt.Errorf("empty caveat")
	}
	i := strings.IndexByte(cav, ' ')
	if i < 0 {
		return cav, "", nil
	}
	if i == 0 {
		return "", "", fmt.Errorf("caveat starts with space character")
	}
	return cav[0:i], cav[i+1:], nil
}

// ClientIPAddrCaveat returns a caveat that will check whether the
// client's IP address is as provided.
// Note that the checkers package provides no specific
// implementation of the checker for this - that is
// left to external transport-specific packages.
func ClientIPAddrCaveat(addr net.IP) Caveat {
	if len(addr) != net.IPv4len && len(addr) != net.IPv6len {
		return ErrorCaveatf("bad IP address %d", []byte(addr))
	}
	return firstParty(CondClientIPAddr, addr.String())
}

// ClientOriginCaveat returns a caveat that will check whether the
// client's Origin header in its HTTP request is as provided.
func ClientOriginCaveat(origin string) Caveat {
	return firstParty(CondClientOrigin, origin)
}

// ErrorCaveatf returns a caveat that will never be satisfied, holding
// the given fmt.Sprintf formatted text as the text of the caveat.
//
// This should only be used for highly unusual conditions that are never
// expected to happen in practice, such as a malformed key that is
// conventionally passed as a constant. It's not a panic but you should
// only use it in cases where a panic might possibly be appropriate.
//
// This mechanism means that caveats can be created without error
// checking and a later systematic check at a higher level (in the
// bakery package) can produce an error instead.
func ErrorCaveatf(f string, a ...interface{}) Caveat {
	return firstParty(CondError, fmt.Sprintf(f, a...))
}

// AllowCaveat returns a caveat that will deny attempts to use the
// macaroon to perform any operation other than those listed. Operations
// must not contain a space.
func AllowCaveat(op ...string) Caveat {
	if len(op) == 0 {
		return ErrorCaveatf("no operations allowed")
	}
	return operationCaveat(CondAllow, op)
}

// DenyCaveat returns a caveat that will deny attempts to use the
// macaroon to perform any of the listed operations. Operations
// must not contain a space.
func DenyCaveat(op ...string) Caveat {
	return operationCaveat(CondDeny, op)
}

// operationCaveat is a helper for AllowCaveat and DenyCaveat. It checks
// that all operation names are valid before createing the caveat.
func operationCaveat(cond string, op []string) Caveat {
	for _, o := range op {
		if strings.IndexByte(o, ' ') != -1 {
			return ErrorCaveatf("invalid operation name %q", o)
		}
	}
	return firstParty(cond, strings.Join(op, " "))
}

// OperationsChecker checks any allow or deny caveats
// with respect to all the named operations in the slice.
// An allow caveat must allow all the operations in the
// slice; a deny caveat will fail if it denies any operation in the
// slice.
type OperationsChecker []string

// Condition implements Checker.Condition.
func (OperationsChecker) Condition() string {
	return ""
}

// Check implements Checker.Check.
func (os OperationsChecker) Check(cond, arg string) error {
	if len(os) == 0 {
		switch cond {
		case CondDeny:
			return nil
		case CondAllow:
			f := strings.Fields(arg)
			if len(f) == 0 {
				return errgo.New("no operations allowed")
			}
			return errgo.Newf("%s not allowed", f[0])
		default:
			return ErrCaveatNotRecognized
		}
	}
	for _, o := range os {
		if err := OperationChecker(o).Check(cond, arg); err != nil {
			return errgo.Mask(err, errgo.Is(ErrCaveatNotRecognized))
		}
	}
	return nil
}

// OperationChecker checks any allow or deny caveats, ensuring they do not
// prohibit the named operation.
type OperationChecker string

// Condition implements Checker.Condition.
func (OperationChecker) Condition() string {
	return ""
}

// Check implements Checker.Check.
func (o OperationChecker) Check(cond, arg string) error {
	var expect bool
	switch cond {
	case CondAllow:
		expect = true
		fallthrough
	case CondDeny:
		var found bool
		for _, op := range strings.Fields(arg) {
			if string(o) == op {
				found = true
				break
			}
		}
		if found == expect {
			return nil
		}
		return fmt.Errorf("%s not allowed", o)
	default:
		return ErrCaveatNotRecognized
	}
}

var _ Checker = OperationChecker("")
