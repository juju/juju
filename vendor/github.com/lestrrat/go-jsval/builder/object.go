package builder

import (
	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
	"github.com/lestrrat/go-pdebug"
)

func buildObjectConstraint(ctx *buildctx, c *jsval.ObjectConstraint, s *schema.Schema) error {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START ObjectConstraint.FromSchema")
		defer g.IRelease("END ObjectConstraint.FromSchema")
	}

	if l := s.Required; len(l) > 0 {
		c.Required(l...)
	}

	if s.MinProperties.Initialized {
		c.MinProperties(s.MinProperties.Val)
	}

	if s.MaxProperties.Initialized {
		c.MaxProperties(s.MaxProperties.Val)
	}

	for pname, pdef := range s.Properties {
		cprop, err := buildFromSchema(ctx, pdef)
		if err != nil {
			return err
		}

		c.AddProp(pname, cprop)
	}

	for rx, pdef := range s.PatternProperties {
		cprop, err := buildFromSchema(ctx, pdef)
		if err != nil {
			return err
		}
		c.PatternProperties(rx, cprop)
	}

	if aprops := s.AdditionalProperties; aprops != nil {
		if sc := aprops.Schema; sc != nil {
			aitem, err := buildFromSchema(ctx, sc)
			if err != nil {
				return err
			}
			c.AdditionalProperties(aitem)
		} else {
			c.AdditionalProperties(jsval.EmptyConstraint)
		}
	}

	for from, to := range s.Dependencies.Names {
		c.PropDependency(from, to...)
	}

	if depschemas := s.Dependencies.Schemas; len(depschemas) > 0 {
		for pname, depschema := range depschemas {
			depc, err := buildFromSchema(ctx, depschema)
			if err != nil {
				return err
			}

			c.SchemaDependency(pname, depc)
		}
	}

	return nil
}
