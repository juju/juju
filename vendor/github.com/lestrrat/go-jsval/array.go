package jsval

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/lestrrat/go-pdebug"
)

// Array creates a new ArrayConstraint
func Array() *ArrayConstraint {
	return &ArrayConstraint{
		additionalItems: EmptyConstraint,
		maxItems:        -1,
		minItems:        -1,
	}
}

// Validate validates the given value against this Constraint
func (c *ArrayConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START ArrayConstraint.Validate")
		defer func() {
			if err == nil {
				g.IRelease("END ArrayConstraint.Validate (PASS)")
			} else {
				g.IRelease("END ArrayConstraint.Validate (FAIL): %s", err)
			}
		}()
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
	default:
		var typ string
		if rv == zeroval {
			typ = "invalid"
		} else {
			typ = rv.Type().String()
		}
		return errors.New("value must be a slice (was: " + typ + ")")
	}

	l := rv.Len()

	if mi := c.minItems; mi > -1 && l < mi {
		return errors.New("fewer items than minItems")
	}

	if mi := c.maxItems; mi > -1 && l > mi {
		return errors.New("more items than maxItems")
	}

	var uitems map[string]struct{}
	if c.uniqueItems {
		pdebug.Printf("Check for unique items enabled")
		uitems = make(map[string]struct{})
		for i := 0; i < l; i++ {
			iv := rv.Index(i).Interface()
			kv := fmt.Sprintf("%s", iv)
pdebug.Printf("unique? -> %s", kv)
			if _, ok := uitems[kv]; ok {
				return errors.New("duplicate element found")
			}
			uitems[kv] = struct{}{}
		}
	}

	if celem := c.items; celem != nil {
		if pdebug.Enabled {
			pdebug.Printf("Checking if all items match a spec")
		}
		// if this is set, then all items must fulfill this.
		// additional items are ignored
		for i := 0; i < l; i++ {
			iv := rv.Index(i).Interface()
			if err := celem.Validate(iv); err != nil {
				return err
			}
		}
	} else {
		// otherwise, check the positional specs, and apply the
		// additionalItems constraint
		lv := rv.Len()
		for i, cpos := range c.positionalItems {
			if lv <= i {
				break
			}
			if pdebug.Enabled {
				pdebug.Printf("Checking positional item at '%d'", i)
			}
			iv := rv.Index(i).Interface()
			if err := cpos.Validate(iv); err != nil {
				return err
			}
		}

		lp := len(c.positionalItems)
		if lp > 0 && l > lp { // we got more than positional schemas
			cadd := c.additionalItems
			if cadd == nil { // you can't have additionalItems!
				return errors.New("additional elements found in array")
			}
			for i := lp - 1; i < l; i++ {
				iv := rv.Index(i).Interface()
				if err := cadd.Validate(iv); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AdditionalItems specifies the constraint that additional items
// must be validated against. Note that if you specify `Items`
// with this constraint, `AdditionalItems` has no effect. This
// constraint only makes sense when used along side with the
// `PositionalItems` constraint.
//
// If you want to allow any additional items, pass `EmptyConstraint`
// to this method. If unspecified, no extra items are allowed.
func (c *ArrayConstraint) AdditionalItems(ac Constraint) *ArrayConstraint {
	c.additionalItems = ac
	return c
}

// Items specifies the constraint that all items in the array
// must be validated against
func (c *ArrayConstraint) Items(ac Constraint) *ArrayConstraint {
	c.items = ac
	return c
}

// MinItems specifies the minimum number of items in the value.
// If unspecified, the check is not performed.
func (c *ArrayConstraint) MinItems(i int) *ArrayConstraint {
	c.minItems = i
	return c
}

// MaxItems specifies the maximum number of items in the value.
// If unspecified, the check is not performed.
func (c *ArrayConstraint) MaxItems(i int) *ArrayConstraint {
	c.maxItems = i
	return c
}

// PositionalItems specifies the constraint that elements in
// each position of the array being validated. Extra items in
// the array are validated against the constraint specified for
// `AdditionalItems`.
func (c *ArrayConstraint) PositionalItems(ac []Constraint) *ArrayConstraint {
	c.positionalItems = ac
	return c
}

// UniqueItems specifies if the array can hold non-unique items.
// When set to true, the validation will fail unless all of your
// elements are unique.
//
// Caveat: Currently we stringify the elements before comparing
// for uniqueness. This could/should be fixed.
func (c *ArrayConstraint) UniqueItems(b bool) *ArrayConstraint {
	if pdebug.Enabled {
		pdebug.Printf("Setting uniqueItems = %t", b)
	}
	c.uniqueItems = b
	return c
}