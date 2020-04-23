// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota

// Checker is implemented by types that can perform quota limit checks.
type Checker interface {
	Check(interface{})
	Outcome() error
}

var _ Checker = (*MultiChecker)(nil)

// MultiChecker composes a list of individual Checker instances.
type MultiChecker struct {
	checkers []Checker
}

// NewMultiChecker returns a Checker that composes the Check/Outcome logic for
// the specified list of Checkers.
func NewMultiChecker(checkers ...Checker) *MultiChecker {
	return &MultiChecker{
		checkers: checkers,
	}
}

// Check passes v to the Check method for each one of the composed Checkers.
func (c MultiChecker) Check(v interface{}) {
	for _, checker := range c.checkers {
		checker.Check(v)
	}
}

// Outcome invokes Outcome on each composed Checker and returns back any
// obtained error or nil if all checks succeeded.
func (c MultiChecker) Outcome() error {
	for _, checker := range c.checkers {
		if err := checker.Outcome(); err != nil {
			return err
		}
	}
	return nil
}
