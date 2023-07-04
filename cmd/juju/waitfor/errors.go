package waitfor

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/juju/waitfor/query"
)

func HelpDisplay(err error, input []rune) error {
	cause := errors.Cause(err)
	if !query.IsSyntaxError(cause) {
		return err
	}

	var builder strings.Builder

	syntaxErr := cause.(*query.SyntaxError)

	builder.WriteString("Cannot parse query:")
	builder.WriteString(" ")
	if len(syntaxErr.Expectations) == 0 {
		builder.WriteString("unexpected character")
	} else if exp := syntaxErr.Expectations[0]; exp == query.STRING {
		builder.WriteString("string not correctly terminated")
	} else {
		builder.WriteString("expected ")
		builder.WriteString(syntaxErr.Expectations[0].String())
	}

	builder.WriteString(".")
	builder.WriteString("\n")
	builder.WriteString("\n")

	ledger := fmt.Sprintf("%d | ", syntaxErr.Pos.Line)
	builder.WriteString(ledger)

	builder.WriteString(getLine(string(input), syntaxErr.Pos.Line))
	builder.WriteString("\n")
	builder.WriteString(strings.Repeat(" ", len(ledger)+(syntaxErr.Pos.Column-1)))
	builder.WriteString(strings.Repeat("^", syntaxErr.Pos.Offset))
	builder.WriteString("\n")

	if len(syntaxErr.Expectations) == 0 {
		builder.WriteString(fmt.Sprintf("wait-for doesn't support %q. Maybe try removing the character and try again.", input[syntaxErr.Pos.Column-1]))
	} else if exp := syntaxErr.Expectations[0]; exp == query.STRING {
		builder.WriteString("Try adding a closing quote to the string.")
	} else {
		builder.WriteString("The type doesn't match the expected syntax. Maybe try removing the character and try again.")
	}

	return fmt.Errorf(builder.String())
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
