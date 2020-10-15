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

// FuncScope is used to call functions for a given identifer.
type FuncScope interface {
	Call(*Identifier, []Ord) (interface{}, error)
}

// BuiltinsRun runs the query with a set of builtin functions.
func (q Query) BuiltinsRun(scope Scope) (bool, error) {
	return q.Run(NewGlobalFuncScope(), scope)
}

// Run the query over a given scope.
func (q Query) Run(fnScope FuncScope, scope Scope) (bool, error) {
	res, err := q.run(q.ast, fnScope, scope)
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

func (q Query) run(e Expression, fnScope FuncScope, scope Scope) (interface{}, error) {
	switch node := e.(type) {
	case *QueryExpression:
		for _, exp := range node.Expressions {
			result, err := q.run(exp, fnScope, scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if result != nil {
				return result, nil
			}
		}
		return nil, nil

	case *ExpressionStatement:
		return q.run(node.Expression, fnScope, scope)

	case *CallExpression:
		fn, ok := node.Name.(*Identifier)
		if !ok {
			return nil, RuntimeErrorf("%s %v unexpected function name", shadowType(node.Name.(Ord)), node.Name.Pos())
		}
		var args []Ord
		for _, arg := range node.Arguments {
			result, err := q.run(arg, fnScope, scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ord, err := liftRawResult(result)
			if err != nil {
				return nil, errors.Trace(err)
			}
			args = append(args, ord)
		}
		res, err := fnScope.Call(fn, args)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return liftRawResult(res)

	case *IndexExpression:
		left, err := q.run(node.Left, fnScope, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}

		index, err := q.run(node.Index, fnScope, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}

		switch t := left.(type) {
		case *OrdMapStringInterface:
			idx, err := expectStringIndex(index)
			if err != nil {
				return nil, errors.Annotatef(err, "%s %v accessing map", shadowType(t), node.Left.Pos())
			}
			res, ok := t.value[idx.value]
			if !ok {
				return nil, RuntimeErrorf("%s %v unexpected index %v accessing map", shadowType(t), node.Left.Pos(), idx.value)
			}
			return liftRawResult(res)

		case *OrdMapInterfaceInterface:
			idx, err := expectOrdIndex(index)
			if err != nil {
				return nil, errors.Annotatef(err, "%s %v accessing map", shadowType(t), node.Left.Pos())
			}
			res, ok := t.value[idx.Value()]
			if !ok {
				return nil, RuntimeErrorf("%s %v unexpected index %v accessing map", shadowType(t), node.Left.Pos(), idx.Value())
			}
			return liftRawResult(res)

		default:
			return nil, RuntimeErrorf("%T %v unexpected index expression", left, node.Left.Pos())
		}

	case *InfixExpression:
		left, err := q.run(node.Left, fnScope, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}

		var right interface{}
		switch node.Token.Type {
		case CONDAND, CONDOR:
			// Don't compute the right handside for a logical operator.
		default:
			right, err = q.run(node.Right, fnScope, scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
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
			return nil, RuntimeErrorf("%T %v logical AND only allowed on boolean values", left, node.Left.Pos())
		}

		// Ensure we don't call the right hand expression unless we need to.
		if node.Token.Type == CONDAND {
			if !leftOp {
				return false, nil
			}
		} else if node.Token.Type == CONDOR {
			if leftOp {
				return true, nil
			}
		} else {
			return nil, RuntimeErrorf("%v unexpected operator %s", node.Token.Pos, node.Token.Literal)
		}

		right, err = q.run(node.Right, fnScope, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}

		switch op := right.(type) {
		case *OrdBool:
			rightOp = op.value
		case bool:
			rightOp = op
		default:
			return nil, RuntimeErrorf("%T %v logical AND only allowed on boolean values", right, node.Right.Pos())
		}

		return rightOp, nil

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
	return nil, RuntimeErrorf("Syntax Error: Unexpected expression %T", e)
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

func liftRawResult(value interface{}) (Ord, error) {
	if ord, ok := value.(Ord); ok {
		return ord, nil
	}

	switch t := value.(type) {
	case string:
		return NewString(t), nil
	case int:
		return NewInteger(int64(t)), nil
	case int64:
		return NewInteger(t), nil
	case bool:
		return NewBool(t), nil
	case float64:
		return NewFloat(t), nil
	case map[interface{}]interface{}:
		return NewMapInterfaceInterface(t), nil
	case map[string]interface{}:
		return NewMapStringInterface(t), nil
	}
	return nil, RuntimeErrorf("%v unexpected index type %T", value, value)
}
