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

	// GetIdents returns the identifers that are supported for a given scope.
	GetIdents() []string

	// GetIdentValue returns the value of the identifier in a given scope.
	GetIdentValue(string) (Box, error)
}

// FuncScope is used to call functions for a given identifer.
type FuncScope interface {
	Add(string, interface{})
	Call(*Identifier, []Box) (interface{}, error)
}

// BuiltinsRun runs the query with a set of builtin functions.
func (q Query) BuiltinsRun(scope Scope) (bool, error) {
	return q.Run(NewGlobalFuncScope(scope), scope)
}

// Run the query over a given scope.
func (q Query) Run(fnScope FuncScope, scope Scope) (bool, error) {
	// Useful for debugging.
	// fmt.Println(q.ast)

	res, err := q.run(q.ast, fnScope, scope)
	if err != nil {
		return false, errors.Trace(err)
	}

	// Attempt to workout if the result of the query is a boolean. This is a bit
	// harder in go as we might have a lot of types that could be returned.
	if res == nil {
		return false, nil
	}
	if box, ok := res.(Box); ok {
		return !box.IsZero(), nil
	}
	ref := reflect.ValueOf(res)
	return !ref.IsZero(), nil
}

func (q Query) run(e Expression, fnScope FuncScope, scope Scope) (interface{}, error) {
	// Useful for debugging.
	// fmt.Printf("%[1]T %[1]v\n", e)

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
			if box, ok := node.Name.(Box); ok {
				return nil, RuntimeErrorf("%s %v unexpected function name", shadowType(box), node.Name.Pos())
			}
			return nil, RuntimeErrorf("%v unexpected function name", node.Name.Pos())
		}
		var args []Box
		for _, arg := range node.Arguments {
			result, err := q.run(arg, fnScope, scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
			box, err := ConvertRawResult(result)
			if err != nil {
				return nil, errors.Trace(err)
			}
			args = append(args, box)
		}
		res, err := fnScope.Call(fn, args)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ConvertRawResult(res)

	case *LambdaExpression:
		arg, ok := node.Argument.(*Identifier)
		if !ok {
			if box, ok := node.Argument.(Box); ok {
				return nil, RuntimeErrorf("%s %v unexpected argument", shadowType(box), node.Argument.Pos())
			}
			return nil, RuntimeErrorf("%v unexpected argument", node.Argument.Pos())
		}

		return NewLambda(arg, func(scope Scope) ([]Box, error) {
			var results []Box
			for _, expr := range node.Expressions {
				result, err := q.run(expr, fnScope, scope)
				if err != nil {
					return nil, errors.Trace(err)
				}
				box, err := ConvertRawResult(result)
				if err != nil {
					return nil, errors.Trace(err)
				}
				results = append(results, box)
			}
			return results, nil
		}), nil

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
		case *BoxMapStringInterface:
			idx, err := expectStringIndex(index)
			if err != nil {
				return nil, errors.Annotatef(err, "%s %v accessing map", shadowType(t), node.Left.Pos())
			}
			res, ok := t.value[idx.value]
			if !ok {
				return nil, RuntimeErrorf("%s %v unexpected index %v accessing map", shadowType(t), node.Left.Pos(), idx.value)
			}
			return ConvertRawResult(res)

		case *BoxMapInterfaceInterface:
			idx, err := expectBoxIndex(index)
			if err != nil {
				return nil, errors.Annotatef(err, "%s %v accessing map", shadowType(t), node.Left.Pos())
			}
			res, ok := t.value[idx.Value()]
			if !ok {
				return nil, RuntimeErrorf("%s %v unexpected index %v accessing map", shadowType(t), node.Left.Pos(), idx.Value())
			}
			return ConvertRawResult(res)

		case *BoxSliceString:
			idx, err := expectIntegerIndex(index)
			if err != nil {
				return nil, errors.Annotatef(err, "%s %v accessing slice", shadowType(t), node.Left.Pos())
			}
			num := int(idx.Value().(int64))
			if num < 0 || num >= len(t.value) {
				return nil, RuntimeErrorf("%s %v range error accessing slice", shadowType(t), node.Left.Pos(), num)
			}
			return ConvertRawResult(t.value[num])

		default:
			return nil, RuntimeErrorf("%T %v unexpected index expression", left, node.Left.Pos())
		}

	case *AccessorExpression:
		parent, err := q.getName(node.Left, fnScope, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}
		child, err := q.getName(node.Right, fnScope, scope)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return scope.GetIdentValue(fmt.Sprintf("%s.%s", parent, child))

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
		case *BoxBool:
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
		case *BoxBool:
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
		return &BoxInteger{value: node.Value}, nil

	case *Float:
		return &BoxFloat{value: node.Value}, nil

	case *String:
		return &BoxString{value: node.Token.Literal}, nil

	case *Bool:
		return &BoxBool{value: node.Value}, nil

	case *Empty:
		return nil, nil
	}
	return nil, RuntimeErrorf("Syntax Error: Unexpected expression %T", e)
}

func (q *Query) getName(node Expression, fnScope FuncScope, scope Scope) (string, error) {
	parent, ok := node.(*Identifier)
	if ok {
		return parent.Token.Literal, nil
	}

	box, err := q.run(node, fnScope, scope)
	if err != nil {
		return "", errors.Trace(err)
	}
	b, ok := box.(Box)
	if !ok {
		return "", RuntimeErrorf("%T %v unexpected identifier", node, node.Pos())
	}
	raw, ok := b.Value().(string)
	if !ok {
		return "", RuntimeErrorf("%T %v unexpected name type", node, node.Pos())
	}
	return raw, nil
}

func equality(left, right interface{}) bool {
	a, ok1 := left.(Box)
	b, ok2 := right.(Box)

	if !ok1 || !ok2 {
		return a == b
	}
	return a.Equal(b)
}

func lessThan(left, right interface{}) bool {
	a, ok1 := left.(Box)
	b, ok2 := right.(Box)

	if !ok1 || !ok2 {
		return false
	}

	return a.Less(b)
}

func lessThanOrEqual(left, right interface{}) bool {
	a, ok1 := left.(Box)
	b, ok2 := right.(Box)

	if !ok1 || !ok2 {
		return false
	}

	return a.Less(b) || a.Equal(b)
}

func valid(o Box) bool {
	switch o := o.(type) {
	case *BoxInteger:
		return o.value > 0
	}
	return false
}

func ConvertRawResult(value interface{}) (Box, error) {
	if box, ok := value.(Box); ok {
		return box, nil
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
	case []string:
		return NewSliceString(t), nil
	}

	return nil, RuntimeErrorf("%v unexpected index type %T", value, value)
}
