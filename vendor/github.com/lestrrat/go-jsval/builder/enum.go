package builder

import (
	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
)

func buildEnumConstraint(_ *buildctx, c *jsval.EnumConstraint, s *schema.Schema) error {
	l := make([]interface{}, len(s.Enum))
	copy(l, s.Enum)
	c.Enum(l)
	return nil
}
