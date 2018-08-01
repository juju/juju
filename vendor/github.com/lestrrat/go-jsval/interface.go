package jsval

import (
	"reflect"
	"regexp"
	"sync"

	"github.com/lestrrat/go-jsref"
)

var zeroval = reflect.Value{}

// JSVal is the main validator object.
type JSVal struct {
	*ConstraintMap
	// Name is the name that will be used to generate code for this validator.
	// If unspecified, the generator will create variable names like `V0`, `V1`,
	// `V2`, etc. If you want to generate more meaningful names, you should
	// set this value manually. For example, if you are using jsval with a
	// scaffold generator, you might want to set this to a human-readable value
	Name     string
	root     Constraint
	resolver *jsref.Resolver
}

// JSValSlice is a list of JSVal validators. This exists in order to define
// methods to satisfy the `sort.Interface` interface
type JSValSlice []*JSVal

// Constraint is an object that know how to validate
// individual types of input
type Constraint interface {
	DefaultValue() interface{}
	HasDefault() bool
	Validate(interface{}) error
}

type emptyConstraint struct{}

// EmptyConstraint is a constraint that returns true for any value
var EmptyConstraint = emptyConstraint{}

type nullConstraint struct{}

// NullConstraint is a constraint that only matches the JSON
// "null" value, or "nil" in golang
var NullConstraint = nullConstraint{}

type defaultValue struct {
	initialized bool
	value       interface{}
}

// BooleanConstraint implements a constraint to match against
// a boolean.
type BooleanConstraint struct {
	defaultValue
}

// StringConstraint implements a constraint to match against
// a string
type StringConstraint struct {
	defaultValue
	enums     *EnumConstraint
	maxLength int
	minLength int
	regexp    *regexp.Regexp
	format    string
}

// NumberConstraint implements a constraint to match against
// a number (i.e. float) type.
type NumberConstraint struct {
	defaultValue
	applyMinimum     bool
	applyMaximum     bool
	applyMultipleOf  bool
	minimum          float64
	maximum          float64
	multipleOf       float64
	exclusiveMinimum bool
	exclusiveMaximum bool
	enums            *EnumConstraint
}

// IntegerConstraint implements a constraint to match against
// an integer type. Note that after checking if the value is
// an int (i.e. floor(x) == x), the rest of the logic follows
// exactly that of NumberConstraint
type IntegerConstraint struct {
	NumberConstraint
}

// ArrayConstraint implements a constraint to match against
// various aspects of a Array/Slice structure
type ArrayConstraint struct {
	defaultValue
	items           Constraint
	positionalItems []Constraint
	additionalItems Constraint
	minItems        int
	maxItems        int
	uniqueItems     bool
}

// ObjectConstraint implements a constraint to match against
// various aspects of a Map-like structure.
type ObjectConstraint struct {
	defaultValue
	additionalProperties Constraint
	deplock              sync.Mutex
	patternProperties    map[*regexp.Regexp]Constraint
	proplock             sync.Mutex
	properties           map[string]Constraint
	propdeps             map[string][]string
	reqlock              sync.Mutex
	required             map[string]struct{}
	maxProperties        int
	minProperties        int
	schemadeps           map[string]Constraint

	// FieldNameFromName takes a struct wrapped in reflect.Value, and a
	// field name -- in JSON format (i.e. what you specified in your
	// JSON struct tags, or the actual field name). It returns the
	// struct field name to pass to `Value.FieldByName()`. If you do
	// not specify one, DefaultFieldNameFromName will be used.
	FieldNameFromName func(reflect.Value, string) string

	// FieldNamesFromStruct takes a struct wrapped in reflect.Value, and
	// returns the name of all public fields. Note that the returned
	// names will be JSON names, which may not necessarily be the same
	// as the field name. If you do not specify one, DefaultFieldNamesFromStruct
	// will be used
	FieldNamesFromStruct func(reflect.Value) []string
}

// EnumConstraint implements a constraint where the incoming
// value must match one of the values enumerated in the constraint.
// Note that due to various language specific reasons, you should
// only use simple types where the "==" operator can distinguish
// between different values
type EnumConstraint struct {
	emptyConstraint
	enums []interface{}
}

type comboconstraint struct {
	emptyConstraint
	constraints []Constraint
}

// AnyConstraint implements a constraint where at least 1
// child constraint must pass in order for this validation to pass.
type AnyConstraint struct {
	comboconstraint
}

// AllConstraint implements a constraint where all of the
// child constraints' validation must pass in order for this
// validation to pass.
type AllConstraint struct {
	comboconstraint
}

// OneOfConstraint implements a constraint where only exactly
// one of the child constraints' validation can pass. If
// none of the constraints passes, or more than 1 constraint
// passes, the expressions deemed to have failed.
type OneOfConstraint struct {
	comboconstraint
}

// NotConstraint implements a constraint where the result of
// child constraint is negated -- that is, validation passes
// only if the child constraint fails.
type NotConstraint struct {
	child Constraint
}
