package builder

import (
	"errors"

	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
	"github.com/lestrrat/go-pdebug"
)

func buildReferenceConstraint(_ *buildctx, r *jsval.ReferenceConstraint, s *schema.Schema) error {
	pdebug.Printf("ReferenceConstraint.buildFromSchema '%s'", s.Reference)
	if s.Reference == "" {
		return errors.New("schema does not contain a reference")
	}
	r.RefersTo(s.Reference)

	return nil
}
