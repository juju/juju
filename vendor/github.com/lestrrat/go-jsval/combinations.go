package jsval

import (
	"errors"

	"github.com/lestrrat/go-pdebug"
)

func (c *comboconstraint) Add(v Constraint) {
	c.constraints = append(c.constraints, v)
}

func (c *comboconstraint) Constraints() []Constraint {
	return c.constraints
}

func reduceCombined(cc interface {
	Constraint
	Constraints() []Constraint
}) Constraint {
	l := cc.Constraints()
	if len(l) == 1 {
		return l[0]
	}
	return cc
}

// Any creates a new AnyConstraint
func Any() *AnyConstraint {
	return &AnyConstraint{}
}

// Reduce returns the child Constraint, if this constraint
// has only 1 child constraint.
func (c *AnyConstraint) Reduce() Constraint {
	return reduceCombined(c)
}

// Add appends a new Constraint
func (c *AnyConstraint) Add(c2 Constraint) *AnyConstraint {
	c.comboconstraint.Add(c2)
	return c
}

// Validate validates the value against the input value.
// For AnyConstraints, it will return success the moment
// one child Constraint succeeds. It will return an error
// if none of the child Constraints succeeds
func (c *AnyConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("AnyConstraint.Validate").BindError(&err)
		defer g.End()
	}
	for _, celem := range c.constraints {
		if err := celem.Validate(v); err == nil {
			return nil
		}
	}
	return errors.New("could not validate against any of the constraints")
}

// All creates a new AllConstraint
func All() *AllConstraint {
	return &AllConstraint{}
}

// Reduce returns the child Constraint, if this constraint
// has only 1 child constraint.
func (c *AllConstraint) Reduce() Constraint {
	return reduceCombined(c)
}

// Add appends a new Constraint
func (c *AllConstraint) Add(c2 Constraint) *AllConstraint {
	c.comboconstraint.Add(c2)
	return c
}

// Validate validates the value against the input value.
// For AllConstraints, it will only return success if
// all of the child Constraints succeeded.
func (c *AllConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("AllConstraint.Validate").BindError(&err)
		defer g.End()
	}

	for _, celem := range c.constraints {
		if err := celem.Validate(v); err != nil {
			return err
		}
	}
	return nil
}

// OneOf creates a new OneOfConstraint
func OneOf() *OneOfConstraint {
	return &OneOfConstraint{}
}

// Reduce returns the child Constraint, if this constraint
// has only 1 child constraint.
func (c *OneOfConstraint) Reduce() Constraint {
	return reduceCombined(c)
}

// Add appends a new Constraint
func (c *OneOfConstraint) Add(c2 Constraint) *OneOfConstraint {
	c.comboconstraint.Add(c2)
	return c
}

// Validate validates the value against the input value.
// For OneOfConstraints, it will return success only if
// exactly 1 child Constraint succeeds.
func (c *OneOfConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("OneOfConstraint.Validate").BindError(&err)
		defer g.End()
	}

	count := 0
	for _, celem := range c.constraints {
		if err := celem.Validate(v); err == nil {
			count++
		}
	}

	if count == 0 {
		return errors.New("none of the constraints passed")
	} else if count > 1 {
		return errors.New("more than 1 of the constraints passed")
	}
	return nil // Yes!
}
