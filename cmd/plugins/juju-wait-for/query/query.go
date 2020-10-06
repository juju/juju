// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"reflect"

	"github.com/juju/errors"
)

// Query holds all the arguments for a given query.
type Query struct {
	ast *QueryExpression
}

// Parse attempts to parse a given query into a argument query.
// Returns an error if it's not in the correct layout.
func Parse(src string) (Query, error) {
	lex := NewLexer(src)
	parser := NewParser(lex)
	ast, err := parser.Run()
	if err != nil {
		return Query{}, errors.Trace(err)
	}

	return Query{
		ast: ast,
	}, nil
}

// Scope is used to identify a given expression of a global mutated object.
type Scope interface {
	// GetIdentValue returns the value of the identifier in a given scope.
	GetIdentValue(string) (Ord, error)
}

// Run the query over a given scope.
func (q Query) Run(scope Scope) (bool, error) {
	res, err := q.run(q.ast, scope)
	if err != nil {
		return false, errors.Trace(err)
	}

	// Attempt to workout if the result of the query is a boolean. This is a bit
	// harder in go as we might have a lot of types that could be returned.
	if res == nil {
		return false, nil
	}
	if ord, ok := res.(Ord); ok {
		return !ord.IsZero(), nil
	}
	ref := reflect.ValueOf(res)
	return !ref.IsZero(), nil
}

func (q Query) run(e Expression, scope Scope) (interface{}, error) {
	switch node := e.(type) {
	case *QueryExpression:
		for _, exp := range node.Expressions {
			result, err := q.run(exp, scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if result != nil {
				return result, nil
			}
		}
		return nil, nil
	case *ExpressionStatement:
		return q.run(node.Expression, scope)
	case *InfixExpression:
		left, err := q.run(node.Left, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}

		right, err := q.run(node.Right, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}

		switch node.Token.Type {
		case EQ:
			return equality(left, right), nil
		case NEQ:
			return !equality(left, right), nil
		case LT:
			return lessThan(left, right), nil
		case LE:
			return lessThanOrEqual(left, right), nil
		case GT:
			return lessThan(right, left), nil
		case GE:
			return lessThanOrEqual(right, left), nil
		}

		// Everything onwards expects to work on logical operators.
		var leftOp, rightOp bool
		switch op := left.(type) {
		case *OrdBool:
			leftOp = op.value
		case bool:
			leftOp = op
		default:
			return nil, errors.Errorf("Runtime Error: %T %v logical AND only allowed on boolean values", left, node.Left.Pos())
		}
		switch op := right.(type) {
		case *OrdBool:
			rightOp = op.value
		case bool:
			rightOp = op
		default:
			return nil, errors.Errorf("Runtime Error: %T %v logical AND only allowed on boolean values", right, node.Right.Pos())
		}

		switch node.Token.Type {
		case CONDAND:
			return leftOp && rightOp, nil
		case CONDOR:
			return leftOp || rightOp, nil
		}

		return nil, errors.Errorf("Runtime Error: %v unexpected operator %s", node.Token.Pos, node.Token.Literal)
	case *Identifier:
		return scope.GetIdentValue(node.Token.Literal)
	case *Integer:
		return &OrdInteger{value: node.Value}, nil
	case *Float:
		return &OrdFloat{value: node.Value}, nil
	case *String:
		return &OrdString{value: node.Token.Literal}, nil
	case *Bool:
		return &OrdBool{value: node.Value}, nil
	case *Empty:
		return nil, nil
	}
	return nil, errors.Errorf("Syntax Error: Unexpected expression %T", e)
}

// Ord represents a ordered datatype.
type Ord interface {
	// Less checks if a Ord is less than another Ord
	Less(Ord) bool

	// Equal checks if an Ord is equal to another Ord.
	Equal(Ord) bool

	// IsZero returns if the underlying value is zero.
	IsZero() bool
}

// OrdInteger defines an ordered integer.
type OrdInteger struct {
	value int64
}

// NewInteger creates a new Ord value
func NewInteger(value int64) *OrdInteger {
	return &OrdInteger{value: value}
}

// Less checks if a OrdInteger is less than another OrdInteger.
func (o *OrdInteger) Less(other Ord) bool {
	if i, ok := other.(*OrdInteger); ok {
		return o.value < i.value
	}
	return false
}

// Equal checks if an OrdInteger is equal to another OrdInteger.
func (o *OrdInteger) Equal(other Ord) bool {
	if i, ok := other.(*OrdInteger); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdInteger) IsZero() bool {
	return o.value < 1
}

// OrdFloat defines an ordered float.
type OrdFloat struct {
	value float64
}

// NewFloat creates a new Ord value
func NewFloat(value float64) *OrdFloat {
	return &OrdFloat{value: value}
}

// Less checks if a OrdFloat is less than another OrdFloat.
func (o *OrdFloat) Less(other Ord) bool {
	if i, ok := other.(*OrdFloat); ok {
		return o.value < i.value
	}
	return false
}

// Equal checks if an OrdFloat is equal to another OrdFloat.
func (o *OrdFloat) Equal(other Ord) bool {
	if i, ok := other.(*OrdFloat); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdFloat) IsZero() bool {
	return o.value < 1
}

// OrdString defines an ordered string.
type OrdString struct {
	value string
}

// NewString creates a new Ord value
func NewString(value string) *OrdString {
	return &OrdString{value: value}
}

// Less checks if a OrdString is less than another OrdString.
func (o *OrdString) Less(other Ord) bool {
	if i, ok := other.(*OrdString); ok {
		return o.value < i.value
	}
	return false
}

// Equal checks if an OrdString is equal to another OrdString.
func (o *OrdString) Equal(other Ord) bool {
	if i, ok := other.(*OrdString); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdString) IsZero() bool {
	return o.value == ""
}

// OrdBool defines an ordered float.
type OrdBool struct {
	value bool
}

// NewBool creates a new Ord value
func NewBool(value bool) *OrdBool {
	return &OrdBool{value: value}
}

// Less checks if a OrdBool is less than another OrdBool.
func (o *OrdBool) Less(other Ord) bool {
	return false
}

// Equal checks if an OrdBool is equal to another OrdBool.
func (o *OrdBool) Equal(other Ord) bool {
	if i, ok := other.(*OrdBool); ok {
		return o.value == i.value
	}
	return false
}

// IsZero returns if the underlying value is zero.
func (o *OrdBool) IsZero() bool {
	return o.value == false
}

func equality(left, right interface{}) bool {
	a, ok1 := left.(Ord)
	b, ok2 := right.(Ord)

	if !ok1 || !ok2 {
		return a == b
	}
	return a.Equal(b)
}

func lessThan(left, right interface{}) bool {
	a, ok1 := left.(Ord)
	b, ok2 := right.(Ord)

	if !ok1 || !ok2 {
		return false
	}

	return a.Less(b)
}

func lessThanOrEqual(left, right interface{}) bool {
	a, ok1 := left.(Ord)
	b, ok2 := right.(Ord)

	if !ok1 || !ok2 {
		return false
	}

	return a.Less(b) || a.Equal(b)
}

func valid(o Ord) bool {
	switch o := o.(type) {
	case *OrdInteger:
		return o.value > 0
	}
	return false
}
