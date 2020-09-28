// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package query

import (
	"strconv"
	"strings"
	"text/scanner"

	"github.com/juju/errors"
)

// Query holds all the arguments for a given query.
type Query struct {
	Arguments map[string][]string
}

// Parse attempts to parse a given query into a argument query.
// Returns an error if it's not in the correct layout.
func Parse(src string) (Query, error) {
	var s scanner.Scanner
	s.Init(strings.NewReader(src))
	s.Filename = "query"

	var tokens []string
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		tokens = append(tokens, s.TokenText())
	}

	// Attempt to parse a very basic AST.
	arguments := make(map[string][]string)
	for i := 0; i < len(tokens); i++ {
		if i+3 > len(tokens) {
			return Query{}, errors.Errorf("unexpected query")
		}

		name := tokens[i]
		if _, ok := arguments[name]; !ok {
			arguments[name] = make([]string, 0)
		}

		i++
		if tokens[i] != "=" {
			return Query{}, errors.Errorf("expected equality sign `=`")
		}

		i++
		if tokens[i] == ";" {
			return Query{}, errors.Errorf("unexpected termination of query")
		}

		// Consume all the tokens available.
		for ; i < len(tokens); i++ {
			if tokens[i] == ";" {
				break
			}
			if tokens[i] == "," {
				continue
			}
			// If we over peek because of a missing ;, then back track to ensure
			// that we can get to the right place.
			if tokens[i] == "=" {
				i -= 2
				arguments[name] = arguments[name][:len(arguments[name])-1]
				break
			}

			value := tokens[i]
			if strings.ContainsAny(value, `"`) {
				var err error
				value, err = strconv.Unquote(tokens[i])
				if err != nil {
					return Query{}, errors.Annotatef(err, "unexpected quotation %q", tokens[i])
				}
			}
			arguments[name] = append(arguments[name], value)
		}
	}

	return Query{
		Arguments: arguments,
	}, nil
}
