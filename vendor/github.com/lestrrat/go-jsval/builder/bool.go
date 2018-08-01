package builder

import (
	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
)

func buildBooleanConstraint(_ *buildctx, c *jsval.BooleanConstraint, s *schema.Schema) error {
	v := s.Default
	switch v.(type) {
	case bool:
		c.Default(v.(bool))
	}
	return nil
}