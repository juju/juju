package jsval

import (
	"errors"
	"sync"

	"github.com/lestrrat/go-pdebug"
)

// RefResolver is a mandatory object that you must pass to a 
// ReferenceConstraint upon its creation. This is responsible
// for resolving the reference to an actual constraint.
type RefResolver interface {
	GetReference(string) (Constraint, error)
}

// ConstraintMap is an implementation of RefResolver
type ConstraintMap struct {
	lock sync.Mutex
	refs map[string]Constraint
}

// Len returns the number of references stored in this ConstraintMap
func (cm ConstraintMap) Len() int {
	return len(cm.refmap())
}

func (cm *ConstraintMap) refmap() map[string]Constraint {
	if cm.refs == nil {
		cm.refs = make(map[string]Constraint)
	}
	return cm.refs
}

// SetReference registeres a new Constraint to a name
func (cm *ConstraintMap) SetReference(name string, c Constraint) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	refs := cm.refmap()
	refs[name] = c
}

// GetReference fetches the Constraint associated with the given name
func (cm *ConstraintMap) GetReference(name string) (Constraint, error) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	refs := cm.refmap()
	c, ok := refs[name]
	if !ok {
		return nil, errors.New("reference '" + name + "' not found")
	}
	return c, nil
}

// ReferenceConstraint is a constraint where its actual definition
// is stored elsewhere.
type ReferenceConstraint struct {
	resolver  RefResolver
	lock      sync.Mutex
	resolved  Constraint
	reference string
}

// Reference creates a new ReferenceConstraint object
func Reference(resolver RefResolver) *ReferenceConstraint {
	return &ReferenceConstraint{
		resolver: resolver,
	}
}

// Resolved returns the Constraint obtained by resolving
// the reference.
func (r *ReferenceConstraint) Resolved() (c Constraint, err error) {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START ReferenceConstraint.Resolved '%s'", r.reference)
		defer func() {
			if err == nil {
				g.IRelease("END ReferenceConstraint.Resolved '%s' (OK)", r.reference)
			} else {
				g.IRelease("END ReferenceConstraint.Resolved '%s' (FAIL): %s", r.reference, err)
			}
		}()
	}
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.resolved != nil {
		if pdebug.Enabled {
			pdebug.Printf("Reference is already resolved")
		}
		return r.resolved, nil
	}

	c, err = r.resolver.GetReference(r.reference)
	if err != nil {
		return nil, err
	}
	r.resolved = c
	return c, nil
}

// RefersTo specifies the reference string that this constraint points to
func (r *ReferenceConstraint) RefersTo(s string) *ReferenceConstraint {
	r.reference = s
	return r
}

// Default is a no op for this type
func (r *ReferenceConstraint) Default(_ interface{}) {
}

// DefaultValue returns the default value from the constraint pointed
// by the reference
func (r *ReferenceConstraint) DefaultValue() interface{} {
	c, err := r.Resolved()
	if err != nil {
		return nil
	}
	return c.DefaultValue()
}

// HasDefault returns true if the constraint pointed by the reference
// has defaults
func (r *ReferenceConstraint) HasDefault() bool {
	c, err := r.Resolved()
	if err != nil {
		return false
	}
	return c.HasDefault()
}

// Validate validates the value against the constraint pointed to
// by the reference.
func (r *ReferenceConstraint) Validate(v interface{}) (err error) {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START ReferenceConstraint.Validate")
		defer func() {
			if err == nil {
				g.IRelease("END ReferenceConstraint.Validate (PASS)")
			} else {
				g.IRelease("END ReferenceConstraint.Validate (FAIL): %s", err)
			}
		}()
	}

	c, err := r.Resolved()
	if err != nil {
		return err
	}
	return c.Validate(v)
}