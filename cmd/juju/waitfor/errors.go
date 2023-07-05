package waitfor

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/juju/waitfor/query"
)

func HelpDisplay(err error, input string) error {
	cause := errors.Cause(err)
	if !query.IsSyntaxError(cause) {
		return err
	}

	var builder strings.Builder

	syntaxErr := cause.(*query.SyntaxError)

	builder.WriteString("Cannot parse query:")
	builder.WriteString(" ")
	builder.WriteString(helpReason(syntaxErr.Expectations))

	builder.WriteString(".")
	builder.WriteString("\n")
	builder.WriteString("\n")

	ledger := fmt.Sprintf("%d | ", syntaxErr.Pos.Line)
	builder.WriteString(ledger)

	builder.WriteString(getLine(input, syntaxErr.Pos.Line))
	builder.WriteString("\n")
	builder.WriteString(strings.Repeat(" ", len(ledger)+(syntaxErr.Pos.Column-1)))

	offset := 1
	if syntaxErr.Pos.Offset > 0 {
		offset = syntaxErr.Pos.Offset
	}
	builder.WriteString(strings.Repeat("^", offset))
	builder.WriteString("\n")

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

func getLine(input string, line int) string {
	scanner := bufio.NewScanner(strings.NewReader(input))
	for i := 0; scanner.Scan(); i++ {
		if i == line-1 {
			return scanner.Text()
		}
	}
	return ""
}
