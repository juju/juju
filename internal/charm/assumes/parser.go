// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package assumes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/semversion"
)

var (
	featureWithoutVersion = regexp.MustCompile(`^[a-z][a-z0-9-]*?[a-z0-9]+$`)
	featureWithVersion    = regexp.MustCompile(`^([a-z][a-z0-9-]*?[a-z0-9]+)\s*?(>=|<)\s*?([\S\.]+)$`)
)

// ExpressionTree is a wrapper for representing a (possibly nested) "assumes"
// block declaration.
type ExpressionTree struct {
	Expression Expression
}

// parseAssumesExpressionTree recursively parses an assumes expression tree
// and returns back an Expression instance for it.
//
// The root of the expression tree consists of a list of (potentially nested)
// assumes expressions that form an implicit All-Of composite expression.
//
// For example:
// assumes:
//   - foo
//   - bar >= 1.42
//   - any-of: ... (nested expr)
//   - all-of: ... (nested expr)
func parseAssumesExpressionTree(rootExprList []interface{}) (Expression, error) {
	var (
		rootExpr = CompositeExpression{
			ExprType:       AllOfExpression,
			SubExpressions: make([]Expression, len(rootExprList)),
		}
		err error
	)

	for i, exprDecl := range rootExprList {
		if rootExpr.SubExpressions[i], err = parseAssumesExpr(exprDecl); err != nil {
			return nil, errors.Annotatef(err, `parsing expression %d in top level "assumes" block`, i+1)
		}
	}

	return rootExpr, nil
}

// parseAssumesExpr returns an Expression instance that corresponds to the
// provided expression declaration. As per the assumes spec, the parser
// supports the following expression types:
//
// 1) feature request expression with optional version constraint (e.g. foo < 1)
// 2) any-of composite expression
// 3) all-of composite expression
func parseAssumesExpr(exprDecl interface{}) (Expression, error) {
	// Is it a composite expression?
	if exprAsMap, isMap := exprDecl.(map[interface{}]interface{}); isMap {
		coercedMap := make(map[string]interface{})
		for key, val := range exprAsMap {
			keyStr, ok := key.(string)
			if !ok {
				return nil, errors.New(`malformed composite expression`)
			}
			coercedMap[keyStr] = val
		}
		return parseCompositeExpr(coercedMap)
	} else if exprAsMap, isMap := exprDecl.(map[string]interface{}); isMap {
		return parseCompositeExpr(exprAsMap)
	}

	// Is it a feature request expression?
	if exprAsString, isString := exprDecl.(string); isString {
		return parseFeatureExpr(exprAsString)
	}

	return nil, errors.New(`expected a feature, "any-of" or "all-of" expression`)
}

// parseCompositeExpr extracts and returns a CompositeExpression from the
// provided expression declaration.
//
// The EBNF grammar for a composite expression is:
//
//	composite-expr-decl: ("any-of"|"all-of") expr-decl-list
//
//	expr-decl-list: expr-decl+
//
//	expr-decl: feature-expr-decl |
//	           composite-expr-decl
//
// The function expects a map with either a "any-of" or "all-of" key and
// a value that is a slice of sub-expressions.
func parseCompositeExpr(exprDecl map[string]interface{}) (CompositeExpression, error) {
	if len(exprDecl) != 1 {
		return CompositeExpression{}, errors.New("malformed composite expression")
	}

	var (
		compositeExpr CompositeExpression
		subExprDecls  interface{}
		err           error
	)

	if subExprDecls = exprDecl["any-of"]; subExprDecls != nil {
		compositeExpr.ExprType = AnyOfExpression
	} else if subExprDecls = exprDecl["all-of"]; subExprDecls != nil {
		compositeExpr.ExprType = AllOfExpression
	} else {
		return CompositeExpression{}, errors.New(`malformed composite expression; expected an "any-of" or "all-of" block`)
	}

	subExprDeclList, isList := subExprDecls.([]interface{})
	if !isList {
		return CompositeExpression{}, errors.Errorf(`malformed %q expression; expected a list of sub-expressions`, string(compositeExpr.ExprType))
	}

	compositeExpr.SubExpressions = make([]Expression, len(subExprDeclList))
	for i, subExprDecl := range subExprDeclList {
		if compositeExpr.SubExpressions[i], err = parseAssumesExpr(subExprDecl); err != nil {
			return CompositeExpression{}, errors.Annotatef(err, "parsing %q expression", string(compositeExpr.ExprType))
		}
	}
	return compositeExpr, nil
}

// parseFeatureExpr extracts and returns a FeatureExpression from the provided
// expression declaration.
//
// The EBNF grammar for feature expressions is:
//
//	feature-expr-decl: feature-ident |
//	                   feature-ident version-constraint version-number
//
//	version-constraint: ">=" | "<"
//	feature-ident: [a-z][a-z0-9-]*[a-z0-9]+
//	version-number: \d+ (‘.’ \d+ (‘.’ \d+)?)?
func parseFeatureExpr(exprDecl string) (FeatureExpression, error) {
	exprDecl = strings.TrimSpace(exprDecl)

	// Is this a feature name without a version constraint?
	if featureWithoutVersion.MatchString(exprDecl) {
		return FeatureExpression{Name: exprDecl}, nil
	}

	matches := featureWithVersion.FindAllStringSubmatch(exprDecl, 1)
	if len(matches) == 1 {
		featName, constraint, versionStr := matches[0][1], matches[0][2], matches[0][3]
		ver, err := semversion.ParseNonStrict(versionStr)
		if err != nil {
			return FeatureExpression{}, errors.Annotatef(err, "malformed feature expression %q", exprDecl)
		}

		return FeatureExpression{
			Name:       featName,
			Constraint: VersionConstraint(constraint),
			Version:    &ver,
			rawVersion: versionStr,
		}, nil
	}

	return FeatureExpression{}, errors.Errorf("malformed feature expression %q", exprDecl)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (tree *ExpressionTree) UnmarshalYAML(unmarshalFn func(interface{}) error) error {
	var exprTree []interface{}
	if err := unmarshalFn(&exprTree); err != nil {
		if _, isTypeErr := err.(*yaml.TypeError); isTypeErr {
			return errors.New(`malformed "assumes" block; expected an expression list`)
		}
		return errors.Annotate(err, "decoding assumes block")
	}

	expr, err := parseAssumesExpressionTree(exprTree)
	if err != nil {
		return errors.Trace(err)
	}
	tree.Expression = expr
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (tree *ExpressionTree) UnmarshalJSON(data []byte) error {
	var exprTree []interface{}
	if err := json.Unmarshal(data, &exprTree); err != nil {
		return errors.Annotate(err, "decoding assumes block")
	}

	expr, err := parseAssumesExpressionTree(exprTree)
	if err != nil {
		return errors.Trace(err)
	}
	tree.Expression = expr
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (tree *ExpressionTree) MarshalYAML() (interface{}, error) {
	if tree == nil || tree.Expression == nil {
		return nil, nil
	}

	return marshalAssumesExpressionTree(tree)
}

// MarshalJSON implements the json.Marshaler interface.
func (tree *ExpressionTree) MarshalJSON() ([]byte, error) {
	if tree == nil || tree.Expression == nil {
		return nil, nil
	}

	exprList, err := marshalAssumesExpressionTree(tree)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return json.Marshal(exprList)
}

func marshalAssumesExpressionTree(tree *ExpressionTree) (interface{}, error) {
	// The root of the expression tree (top level of the assumes block) is
	// always an implicit "any-of".  We need to marshal it into a map and
	// extract the expression list.
	root, err := marshalExpr(tree.Expression)
	if err != nil {
		return nil, err
	}

	rootMap, ok := root.(map[string]interface{})
	if !ok {
		return nil, errors.New(`unexpected serialized output for top-level "assumes" block`)
	}

	exprList, ok := rootMap[string(AllOfExpression)]
	if !ok {
		return nil, errors.New(`unexpected serialized output for top-level "assumes" block`)
	}

	return exprList, nil
}

func marshalExpr(expr Expression) (interface{}, error) {
	featExpr, ok := expr.(FeatureExpression)
	if ok {
		if featExpr.Version == nil {
			return featExpr.Name, nil
		}

		// If we retained the raw version use that; otherwise convert
		// the parsed version to a string.
		if featExpr.rawVersion != "" {
			return fmt.Sprintf("%s %s %s", featExpr.Name, featExpr.Constraint, featExpr.rawVersion), nil
		}

		return fmt.Sprintf("%s %s %s", featExpr.Name, featExpr.Constraint, featExpr.Version.String()), nil
	}

	// This is a composite expression
	compExpr, ok := expr.(CompositeExpression)
	if !ok {
		return nil, errors.Errorf("unexpected expression type %s", expr.Type())
	}

	var (
		exprList = make([]interface{}, len(compExpr.SubExpressions))
		err      error
	)

	for i, subExpr := range compExpr.SubExpressions {
		if exprList[i], err = marshalExpr(subExpr); err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{
		string(compExpr.ExprType): exprList,
	}, nil
}
