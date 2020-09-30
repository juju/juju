// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"fmt"
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
	GetIdentValue(string) (interface{}, error)
}

// Run the query over a given scope.
func (q Query) Run(scope Scope) (bool, error) {
	res, err := q.run(q.ast, scope)
	if err != nil {
		return false, errors.Trace(err)
	}

	// Attempt to workout if the result of the query is a boolean. This is a bit
	// harder in go as we might have a lot of types that could be returned.
	switch r := res.(type) {
	case nil:
		return false, nil
	case bool:
		return r, nil
	case int:
		return r > 0, nil
	case int8:
		return r > 0, nil
	case int16:
		return r > 0, nil
	case int32:
		return r > 0, nil
	case int64:
		return r > 0, nil
	case uint:
		return r > 0, nil
	case uint8:
		return r > 0, nil
	case uint16:
		return r > 0, nil
	case uint32:
		return r > 0, nil
	case uint64:
		return r > 0, nil
	case string:
		return r != "", nil
	}

	return false, errors.Errorf("Runtime Error: unknown query return type %T", res)
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
		}

		// Everything onwards expects to work on logical operators.
		var (
			leftOp  bool
			rightOp bool
			ok      bool
		)
		if leftOp, ok = left.(bool); !ok {
			return nil, errors.Errorf("Runtime Error: %v logical AND only allowed on boolean values", node.Left.Pos())
		}
		if rightOp, ok = right.(bool); !ok {
			return nil, errors.Errorf("Runtime Error: %v logical AND only allowed on boolean values", node.Right.Pos())
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
		return node.Value, nil
	case *Float:
		return node.Value, nil
	case *String:
		return node.Token.Literal, nil
	case *Bool:
		return node.Value, nil
	case *Empty:
		return nil, nil
	}
	return nil, errors.Errorf("Syntax Error: Unexpected expression %T", e)
}

func equality(left, right interface{}) bool {
	if left == right {
		return true
	}
	if reflect.DeepEqual(left, right) {
		return true
	}

	// We might have a shadowed type here, if that's the case let's attempt
	// to expose that.
	leftVal, rightVal := reveal(left), reveal(right)
	if leftVal == rightVal {
		return true
	}
	if reflect.DeepEqual(leftVal, rightVal) {
		return true
	}

	return false
}

func reveal(val interface{}) interface{} {
	t := reflect.TypeOf(val)
	switch t.Kind() {
	case reflect.String:
		return fmt.Sprintf("%s", val)
	}
	return val
}
