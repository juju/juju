package waitfor

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/cmd/juju/waitfor/query"
)

// ColumnPrefix is the prefix used for column errors. By default the prefix
// for all errors from the cmd line is ERROR.
const ColumnPrefix = "ERROR "

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
	} else {
		builder.WriteString("expected ")
		builder.WriteString(syntaxErr.Expectations[0].String())
	}
	builder.WriteString(".")
	builder.WriteString("\n")
	builder.WriteString("\n")

	ledger := fmt.Sprintf("%d | ", syntaxErr.Pos.Line)
	builder.WriteString(ledger)

	builder.WriteString(string(input))
	builder.WriteString("\n")
	builder.WriteString(strings.Repeat(" ", len(ledger)+(syntaxErr.Pos.Column-1)))
	builder.WriteString(strings.Repeat("^", syntaxErr.Pos.Offset))
	builder.WriteString("\n")

	builder.WriteString(fmt.Sprintf("wait-for doesn't support %q. Maybe try removing the character and try again.", input[syntaxErr.Pos.Column-1]))

	return fmt.Errorf(builder.String())
}
