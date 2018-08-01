package builder

/* Package builder contains structures and methods responsible for
 * generating a jsval.JSVal structure from a JSON schema
 */

import (
	"errors"
	"math"
	"reflect"
	"sort"

	"github.com/lestrrat/go-jsref"
	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
	"github.com/lestrrat/go-pdebug"
)

// Builder builds Validator objects from JSON schemas
type Builder struct{}

type buildctx struct {
	V *jsval.JSVal
	S *schema.Schema
	R map[string]struct{}
}

// New creates a new builder object
func New() *Builder {
	return &Builder{}
}

// Build creates a new validator from the specified schema
func (b *Builder) Build(s *schema.Schema) (v *jsval.JSVal, err error) {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START Builder.Build")
		defer func() {
			if err == nil {
				g.IRelease("END Builder.Build (OK)")
			} else {
				g.IRelease("END Builder.Build (FAIL): %s", err)
			}
		}()
	}

	if s == nil {
		return nil, errors.New("nil schema")
	}

	return b.BuildWithCtx(s, nil)
}

// BuildWithCtx creates a new validator from the specified schema, using
// the jsctx parameter as the context to resolve JSON References with.
// If you expect your schema to contain JSON references to itself,
// you will have to pass the context as a map with raw decoded JSON data
func (b *Builder) BuildWithCtx(s *schema.Schema, jsctx interface{}) (v *jsval.JSVal, err error) {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START Builder.BuildWithCtx")
		defer func() {
			if err == nil {
				g.IRelease("END Builder.BuildWithCtx (OK)")
			} else {
				g.IRelease("END Builder.BuildWithCtx (FAIL): %s", err)
			}
		}()
	}

	if s == nil {
		return nil, errors.New("nil schema")
	}

	v = jsval.New()
	ctx := buildctx{
		V: v,
		S: s,
		R: map[string]struct{}{}, // names of references used
	}

	c, err := buildFromSchema(&ctx, s)
	if err != nil {
		return nil, err
	}

	if _, ok := ctx.R["#"]; ok {
		v.SetReference("#", c)
		delete(ctx.R, "#")
	}

	// Now, resolve references that were used in the schema
	if len(ctx.R) > 0 {
		if pdebug.Enabled {
			pdebug.Printf("Checking references now")
		}
		if jsctx == nil {
			jsctx = s
		}

		r := jsref.New()
		for ref := range ctx.R {
			if err := compileReferences(&ctx, r, v, ref, jsctx); err != nil {
				return nil, err
			}
		}
	}
	v.SetRoot(c)
	return v, nil
}

func compileReferences(ctx *buildctx, r *jsref.Resolver, v *jsval.JSVal, ref string, jsctx interface{}) error {
	if _, err := v.GetReference(ref); err == nil {
		if pdebug.Enabled {
			pdebug.Printf("Already resolved constraints for reference '%s'", ref)
		}
		return nil
	}

	if pdebug.Enabled {
		pdebug.Printf("Building constraints for reference '%s'", ref)
	}

	thing, err := r.Resolve(jsctx, ref)
	if err != nil {
		return err
	}

	if pdebug.Enabled {
		pdebug.Printf("'%s' resolves to the main schema", ref)
	}

	var s1 *schema.Schema
	switch thing.(type) {
	case *schema.Schema:
		s1 = thing.(*schema.Schema)
	case map[string]interface{}:
		s1 = schema.New()
		if err := s1.Extract(thing.(map[string]interface{})); err != nil {
			return err
		}
	}

	c1, err := buildFromSchema(ctx, s1)
	if err != nil {
		return err
	}

	v.SetReference(ref, c1)
	for ref := range ctx.R {
		if err := compileReferences(ctx, r, v, ref, jsctx); err != nil {
			return err
		}
	}
	return nil
}

func buildFromSchema(ctx *buildctx, s *schema.Schema) (jsval.Constraint, error) {
	if ref := s.Reference; ref != "" {
		c := jsval.Reference(ctx.V)
		if err := buildReferenceConstraint(ctx, c, s); err != nil {
			return nil, err
		}
		ctx.R[ref] = struct{}{}
		return c, nil
	}

	ct := jsval.All()

	switch {
	case s.Not != nil:
		if pdebug.Enabled {
			pdebug.Printf("Not constraint")
		}
		c1, err := buildFromSchema(ctx, s.Not)
		if err != nil {
			return nil, err
		}
		ct.Add(jsval.Not(c1))
	case len(s.AllOf) > 0:
		if pdebug.Enabled {
			pdebug.Printf("AllOf constraint")
		}
		ac := jsval.All()
		for _, s1 := range s.AllOf {
			c1, err := buildFromSchema(ctx, s1)
			if err != nil {
				return nil, err
			}
			ac.Add(c1)
		}
		ct.Add(ac.Reduce())
	case len(s.AnyOf) > 0:
		if pdebug.Enabled {
			pdebug.Printf("AnyOf constraint")
		}
		ac := jsval.Any()
		for _, s1 := range s.AnyOf {
			c1, err := buildFromSchema(ctx, s1)
			if err != nil {
				return nil, err
			}
			ac.Add(c1)
		}
		ct.Add(ac.Reduce())
	case len(s.OneOf) > 0:
		if pdebug.Enabled {
			pdebug.Printf("OneOf constraint")
		}
		oc := jsval.OneOf()
		for _, s1 := range s.OneOf {
			c1, err := buildFromSchema(ctx, s1)
			if err != nil {
				return nil, err
			}
			oc.Add(c1)
		}
		ct.Add(oc.Reduce())
	}

	var sts schema.PrimitiveTypes
	if l := len(s.Type); l > 0 {
		sts = make(schema.PrimitiveTypes, l)
		copy(sts, s.Type)
	} else {
		if pdebug.Enabled {
			pdebug.Printf("Schema doesn't seem to contain a 'type' field. Now guessing...")
		}
		sts = guessSchemaType(s)
	}
	sort.Sort(sts)

	if len(sts) > 0 {
		tct := jsval.Any()
		for _, st := range sts {
			var c jsval.Constraint
			switch st {
			case schema.StringType:
				sc := jsval.String()
				if err := buildStringConstraint(ctx, sc, s); err != nil {
					return nil, err
				}
				c = sc
			case schema.NumberType:
				nc := jsval.Number()
				if err := buildNumberConstraint(ctx, nc, s); err != nil {
					return nil, err
				}
				c = nc
			case schema.IntegerType:
				ic := jsval.Integer()
				if err := buildIntegerConstraint(ctx, ic, s); err != nil {
					return nil, err
				}
				c = ic
			case schema.BooleanType:
				bc := jsval.Boolean()
				if err := buildBooleanConstraint(ctx, bc, s); err != nil {
					return nil, err
				}
				c = bc
			case schema.ArrayType:
				ac := jsval.Array()
				if err := buildArrayConstraint(ctx, ac, s); err != nil {
					return nil, err
				}
				c = ac
			case schema.ObjectType:
				oc := jsval.Object()
				if err := buildObjectConstraint(ctx, oc, s); err != nil {
					return nil, err
				}
				c = oc
			case schema.NullType:
				c = jsval.NullConstraint
			default:
				return nil, errors.New("unknown type: " + st.String())
			}
			tct.Add(c)
		}
		ct.Add(tct.Reduce())
	} else {
		// All else failed, check if we have some enumeration?
		if len(s.Enum) > 0 {
			ec := jsval.Enum(s.Enum...)
			ct.Add(ec)
		}
	}

	return ct.Reduce(), nil
}

func guessSchemaType(s *schema.Schema) schema.PrimitiveTypes {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START guessSchemaType")
		defer g.IRelease("END guessSchemaType")
	}

	var sts schema.PrimitiveTypes
	if schemaLooksLikeObject(s) {
		if pdebug.Enabled {
			pdebug.Printf("Looks like it could be an object...")
		}
		sts = append(sts, schema.ObjectType)
	}

	if schemaLooksLikeArray(s) {
		if pdebug.Enabled {
			pdebug.Printf("Looks like it could be an array...")
		}
		sts = append(sts, schema.ArrayType)
	}

	if schemaLooksLikeString(s) {
		if pdebug.Enabled {
			pdebug.Printf("Looks like it could be a string...")
		}
		sts = append(sts, schema.StringType)
	}

	if ok, typ := schemaLooksLikeNumber(s); ok {
		if pdebug.Enabled {
			pdebug.Printf("Looks like it could be a number...")
		}
		sts = append(sts, typ)
	}

	if schemaLooksLikeBool(s) {
		if pdebug.Enabled {
			pdebug.Printf("Looks like it could be a bool...")
		}
		sts = append(sts, schema.BooleanType)
	}

	if pdebug.Enabled {
		pdebug.Printf("Guessed types: %#v", sts)
	}

	return sts
}

func schemaLooksLikeObject(s *schema.Schema) bool {
	if len(s.Properties) > 0 {
		return true
	}

	if s.AdditionalProperties == nil {
		return true
	}

	if s.AdditionalProperties.Schema != nil {
		return true
	}

	if s.MinProperties.Initialized {
		return true
	}

	if s.MaxProperties.Initialized {
		return true
	}

	if len(s.Required) > 0 {
		return true
	}

	if len(s.PatternProperties) > 0 {
		return true
	}

	for _, v := range s.Enum {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Map, reflect.Struct:
			return true
		}
	}

	return false
}

func schemaLooksLikeArray(s *schema.Schema) bool {
	if s.Items != nil {
		return true
	}

	if s.AdditionalItems == nil {
		return true
	}

	if s.AdditionalItems.Schema != nil {
		return true
	}

	if s.Items != nil {
		return true
	}

	if s.MinItems.Initialized {
		return true
	}

	if s.MaxItems.Initialized {
		return true
	}

	if s.UniqueItems.Initialized {
		return true
	}

	for _, v := range s.Enum {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice:
			return true
		}
	}

	return false
}

func schemaLooksLikeString(s *schema.Schema) bool {
	if s.MinLength.Initialized {
		return true
	}

	if s.MaxLength.Initialized {
		return true
	}

	if s.Pattern != nil {
		return true
	}

	switch s.Format {
	case schema.FormatDateTime, schema.FormatEmail, schema.FormatHostname, schema.FormatIPv4, schema.FormatIPv6, schema.FormatURI:
		return true
	}

	for _, v := range s.Enum {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.String:
			return true
		}
	}

	return false
}

func isInteger(n schema.Number) bool {
	return math.Floor(n.Val) == n.Val
}

func schemaLooksLikeNumber(s *schema.Schema) (bool, schema.PrimitiveType) {
	if s.MultipleOf.Initialized {
		if isInteger(s.MultipleOf) {
			return true, schema.IntegerType
		}
		return true, schema.NumberType
	}

	if s.Minimum.Initialized {
		if isInteger(s.Minimum) {
			return true, schema.IntegerType
		}
		return true, schema.NumberType
	}

	if s.Maximum.Initialized {
		if isInteger(s.Maximum) {
			return true, schema.IntegerType
		}
		return true, schema.NumberType
	}

	if s.ExclusiveMinimum.Initialized {
		return true, schema.NumberType
	}

	if s.ExclusiveMaximum.Initialized {
		return true, schema.NumberType
	}

	for _, v := range s.Enum {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return true, schema.IntegerType
		case reflect.Float32, reflect.Float64:
			return true, schema.NumberType
		}
	}

	return false, schema.UnspecifiedType
}

func schemaLooksLikeBool(s *schema.Schema) bool {
	for _, v := range s.Enum {
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Bool:
			return true
		}
	}

	return false
}