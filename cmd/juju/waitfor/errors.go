// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"bufio"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/juju/waitfor/query"
)

func HelpDisplay(err error, input string, idents []string) error {
	switch {
	case query.IsSyntaxError(err):
		return syntaxErrDisplay(err, input)
	case query.IsInvalidIdentifierErr(err):
		return invalidIdentifierDisplay(err, input, idents)
	case query.IsRuntimeError(err):
		return runtimeErrDisplay(err, input)
	case query.IsRuntimeSyntaxError(err):
		return runtimeSyntaxErrDisplay(err, input)
	default:
		fmt.Println()
		return err
	}
}

func syntaxErrDisplay(err error, input string) error {
	cause := errors.Cause(err)
	syntaxErr := cause.(*query.SyntaxError)

	var builder strings.Builder
	builder.WriteString("Cannot parse query:")
	builder.WriteString(" ")
	builder.WriteString(helpReason(syntaxErr.Expectations))

	builder.WriteString(".")
	builder.WriteString("\n")
	builder.WriteString("\n")

	builder.WriteString(helpLineError(input, syntaxErr.Pos))

	builder.WriteString(helpMessage(input, syntaxErr.Pos, syntaxErr.Expectations))
	builder.WriteString(".")

	return fmt.Errorf(builder.String())
}

func helpReason(exps []query.TokenType) string {
	if len(exps) == 0 {
		return "unexpected character"
	}
	switch exps[0] {
	case query.STRING:
		return "string is not correctly terminated"
	case query.CONDAND:
		return "invalid AND (&&) operator"
	case query.CONDOR:
		return "invalid OR (||) operator"
	default:
		return fmt.Sprintf("expected %s", exps[0].String())
	}
}

func helpMessage(input string, pos query.Position, exps []query.TokenType) string {
	if len(exps) == 0 {
		return fmt.Sprintf("wait-for doesn't support %q. Maybe try removing the character and try again", input[pos.Column-1])
	}
	switch exps[0] {
	case query.STRING:
		return "Try adding a closing quote to the string"
	case query.CONDAND:
		return "Ensure that each side of the && operator can be evaluated to a boolean"
	case query.CONDOR:
		return "Ensure that each side of the || operator can be evaluated to a boolean"
	default:
		return "The type doesn't match the expected syntax. Maybe try removing the character and try again"
	}
}

func helpLineError(input string, pos query.Position) string {
	var builder strings.Builder
	ledger := fmt.Sprintf("%d | ", pos.Line)
	builder.WriteString(ledger)

	builder.WriteString(getLine(input, pos.Line))
	builder.WriteString("\n")

	leading := len(ledger) + (pos.Column - 1)
	builder.WriteString(strings.Repeat(" ", leading))

	offset := 1
	if pos.Offset > 0 {
		offset = pos.Offset
	}
	if leading+offset > len(input) {
		offset = leading - len(input)
	}
	if offset > 0 {
		builder.WriteString(strings.Repeat("^", offset))
		builder.WriteString("\n")
	}

	return builder.String()
}

func helpLineErrors(input string) string {
	var builder strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(input))
	for i := 0; scanner.Scan(); i++ {
		builder.WriteString(fmt.Sprintf("%d | ", i+1))
		builder.WriteString(scanner.Text())
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
	return builder.String()
}

func getLine(input string, line int) string {
	scanner := bufio.NewScanner(strings.NewReader(input))
	for i := 0; scanner.Scan(); i++ {
		if i == line-1 {
			return scanner.Text()
		}
	}
	return ""
}

func invalidIdentifierDisplay(err error, input string, defaultIdents []string) error {
	cause := errors.Cause(err)
	identErr := cause.(*query.InvalidIdentifierError)

	var builder strings.Builder
	builder.WriteString("Cannot execute query: invalid identifier.")
	builder.WriteString("\n")
	builder.WriteString("\n")

	builder.WriteString(helpLineErrors(input))

	builder.WriteString("No possible matches for")
	builder.WriteString(" ")
	builder.WriteString(err.Error())
	builder.WriteString("\n")

	idents := defaultIdents
	if identErr.Scope() != nil {
		idents = identErr.Scope().GetIdents()
	}

	first, other, ok := orderPotentialMatches(identErr.Name(), idents)
	if !ok {
		return fmt.Errorf(builder.String())
	}

	if first != "" {
		builder.WriteString("Did you mean:")
		builder.WriteString("\n")
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("    %s", first))
		builder.WriteString("\n")
	}

	if len(other) > 0 {
		builder.WriteString("\n")
		builder.WriteString("Other possible matches:")
		builder.WriteString("\n")
		builder.WriteString("\n")
		builder.WriteString("    - ")
		builder.WriteString(strings.Join(other, "\n    - "))
	}

	return fmt.Errorf(builder.String())
}

func orderPotentialMatches(name string, possible []string) (string, []string, bool) {
	// Remove any duplicates.
	possibleSet := set.NewStrings(possible...)
	if possibleSet.Size() == 0 {
		return "", nil, false
	}

	type Indexed = struct {
		Name  string
		Value int
	}

	possible = possibleSet.SortedValues()
	matches := make([]Indexed, 0, len(possible))
	for _, ident := range possible {
		matches = append(matches, Indexed{
			Name:  ident,
			Value: levenshteinDistance(name, ident),
		})
	}
	// Find the smallest levenshtein distance. If two values are the same,
	// fallback to sorting on the name, which should give predictable results.
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Value < matches[j].Value {
			return true
		}
		if matches[i].Value > matches[j].Value {
			return false
		}
		return matches[i].Name < matches[j].Name
	})

	if len(matches) == 0 {
		return "", nil, false
	}

	matchedName := matches[0].Name
	matchedValue := matches[0].Value
	if matchedName != "" && matchedValue <= len(matchedName)+1 {
		others := set.NewStrings(possible...)
		others.Remove(matchedName)
		return matchedName, others.SortedValues(), true
	}

	return "", possible, false
}

func runtimeErrDisplay(err error, input string) error {
	cause := errors.Cause(err)
	runErr := cause.(*query.RuntimeError)

	var builder strings.Builder
	builder.WriteString("Cannot execute query: runtime error.")
	builder.WriteString("\n")
	builder.WriteString("\n")

	builder.WriteString(helpLineErrors(input))

	builder.WriteString("wait-for requires the ")
	builder.WriteString(runErr.Error())
	builder.WriteString("\n")
	builder.WriteString("Maybe try removing the character and try again.")

	return fmt.Errorf(builder.String())
}

func runtimeSyntaxErrDisplay(err error, input string) error {
	cause := errors.Cause(err)
	runErr := cause.(*query.RuntimeSyntaxError)

	var builder strings.Builder
	builder.WriteString("Cannot execute query: runtime syntax error.")
	builder.WriteString("\n")
	builder.WriteString("\n")

	builder.WriteString(helpLineErrors(input))

	builder.WriteString("wait-for has ")
	builder.WriteString(runErr.Error())
	builder.WriteString(".")
	builder.WriteString("\n")

	first, other, ok := orderPotentialMatches(runErr.Name, runErr.Options)
	if !ok {
		return fmt.Errorf(builder.String())
	}

	if first != "" {
		builder.WriteString("Did you mean:")
		builder.WriteString("\n")
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("    %s", first))
		builder.WriteString("\n")
	}

	if len(other) > 0 {
		builder.WriteString("\n")
		builder.WriteString("Other possible matches:")
		builder.WriteString("\n")
		builder.WriteString("\n")
		builder.WriteString("    - ")
		builder.WriteString(strings.Join(other, "\n    - "))
	}

	return fmt.Errorf(builder.String())
}

// levenshteinDistance
// from https://groups.google.com/forum/#!topic/golang-nuts/YyH1f_qCZVc
// (no min, compute lengths once, 2 rows array)
// fastest profiled
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)
	d := make([]int, la+1)
	var lastdiag, olddiag, temp int

	for i := 1; i <= la; i++ {
		d[i] = i
	}
	for i := 1; i <= lb; i++ {
		d[0] = i
		lastdiag = i - 1
		for j := 1; j <= la; j++ {
			olddiag = d[j]
			min := d[j] + 1
			if (d[j-1] + 1) < min {
				min = d[j-1] + 1
			}
			if a[j-1] == b[i-1] {
				temp = 0
			} else {
				temp = 1
			}
			if (lastdiag + temp) < min {
				min = lastdiag + temp
			}
			d[j] = min
			lastdiag = olddiag
		}
	}
	return d[la]
}
