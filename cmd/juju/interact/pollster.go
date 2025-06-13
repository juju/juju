// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package interact

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"golang.org/x/crypto/ssh/terminal"
)

// Non standard json Format
const FormatCertFilename jsonschema.Format = "cert-filename"

// Pollster is used to ask multiple questions of the user using a standard
// formatting.
type Pollster struct {
	VerifyURLs     VerifyFunc
	VerifyCertFile VerifyFunc
	scanner        *bufio.Scanner
	out            io.Writer
	errOut         io.Writer
	in             io.Reader
}

// New returns a Pollster that wraps the given reader and writer.
func New(in io.Reader, out, errOut io.Writer) *Pollster {
	return &Pollster{
		scanner: bufio.NewScanner(byteAtATimeReader{in}),
		out:     out,
		errOut:  errOut,
		in:      in,
	}
}

// List contains the information necessary to ask the user to select one item
// from a list of options.
type List struct {
	Singular string
	Plural   string
	Options  []string
	Default  string
}

// MultiList contains the information necessary to ask the user to select from a
// list of options.
type MultiList struct {
	Singular string
	Plural   string
	Options  []string
	Default  []string
}

var listTmpl = template.Must(template.New("").Funcs(map[string]interface{}{"title": strings.Title}).Parse(`
{{title .Plural}}
{{range .Options}}  {{.}}
{{end}}
`[1:]))

var selectTmpl = template.Must(template.New("").Parse(`
Select {{.Singular}}{{if .Default}} [{{.Default}}]{{end}}: `[1:]))

// Select queries the user to select from the given list of options.
func (p *Pollster) Select(l List) (string, error) {
	return p.SelectVerify(l, VerifyOptions(l.Singular, l.Options, l.Default != ""))
}

// SelectVerify queries the user to select from the given list of options,
// verifying the choice by passing responses through verify.
func (p *Pollster) SelectVerify(l List, verify VerifyFunc) (string, error) {
	if err := listTmpl.Execute(p.out, l); err != nil {
		return "", err
	}

	question, err := sprint(selectTmpl, l)
	if err != nil {
		return "", errors.Trace(err)
	}
	val, err := QueryVerify(question, p.scanner, p.out, p.errOut, verify)
	if err != nil {
		return "", errors.Trace(err)
	}
	if val == "" {
		return l.Default, nil
	}
	return val, nil
}

var multiSelectTmpl = template.Must(template.New("").Funcs(
	map[string]interface{}{"join": strings.Join}).Parse(`
Select one or more {{.Plural}} separated by commas{{if .Default}} [{{join .Default ", "}}]{{end}}: `[1:]))

// MultiSelect queries the user to select one more answers from the given list of
// options by entering values delimited by commas (and thus options must not
// contain commas).
func (p *Pollster) MultiSelect(l MultiList) ([]string, error) {
	var bad []string
	for _, s := range l.Options {
		if strings.Contains(s, ",") {
			bad = append(bad, s)
		}
	}
	if len(bad) > 0 {
		return nil, errors.Errorf("options may not contain commas: %q", bad)
	}
	if err := listTmpl.Execute(p.out, l); err != nil {
		return nil, err
	}

	// If there is only ever one option and that option also equals the default
	// option, then just echo out what that option is (above), then return that
	// option back.
	if len(l.Default) == 1 && len(l.Options) == 1 && l.Options[0] == l.Default[0] {
		return l.Default, nil
	}

	question, err := sprint(multiSelectTmpl, l)
	if err != nil {
		return nil, errors.Trace(err)
	}
	verify := multiVerify(l.Singular, l.Plural, l.Options, l.Default != nil)
	a, err := QueryVerify(question, p.scanner, p.out, p.errOut, verify)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if a == "" {
		return l.Default, nil
	}
	return multiSplit(a), nil
}

// Enter requests that the user enter a value.  Any value except an empty string
// is accepted.
func (p *Pollster) Enter(valueName string) (string, error) {
	return p.EnterVerify(valueName, func(s string) (ok bool, msg string, err error) {
		return s != "", "", nil
	})
}

// EnterPassword works like Enter except that if the pollster's input wraps a
// terminal, the user's input will be read without local echo.
func (p *Pollster) EnterPassword(valueName string) (string, error) {
	if f, ok := p.in.(*os.File); ok && terminal.IsTerminal(int(f.Fd())) {
		defer fmt.Fprint(p.out, "\n\n")
		if _, err := fmt.Fprintf(p.out, "Enter %s: ", valueName); err != nil {
			return "", errors.Trace(err)
		}
		value, err := terminal.ReadPassword(int(f.Fd()))
		if err != nil {
			return "", errors.Trace(err)
		}
		return string(value), nil
	}
	return p.Enter(valueName)
}

// EnterDefault requests that the user enter a value.  Any value is accepted.
// An empty string is treated as defVal.
func (p *Pollster) EnterDefault(valueName, defVal string) (string, error) {
	return p.EnterVerifyDefault(valueName, nil, defVal)
}

// VerifyFunc is a type that determines whether a value entered by the user is
// acceptable or not.  If it returns an error, the calling func will return an
// error, and the other return values are ignored.  If ok is true, the value is
// acceptable, and that value will be returned by the calling function.  If ok
// is false, the user will be asked to enter a new value for query.  If ok is
// false. if errmsg is not empty, it will be printed out as an error to te the
// user.
type VerifyFunc func(s string) (ok bool, errmsg string, err error)

// EnterVerify requests that the user enter a value.  Values failing to verify
// will be rejected with the error message returned by verify.  A nil verify
// function will accept any value (even an empty string).
func (p *Pollster) EnterVerify(valueName string, verify VerifyFunc) (string, error) {
	return QueryVerify("Enter "+valueName+": ", p.scanner, p.out, p.errOut, verify)
}

// EnterOptional requests that the user enter a value.  It accepts any value,
// even an empty string.
func (p *Pollster) EnterOptional(valueName string) (string, error) {
	return QueryVerify("Enter "+valueName+" (optional): ", p.scanner, p.out, p.errOut, nil)
}

// EnterVerifyDefault requests that the user enter a value.  Values failing to
// verify will be rejected with the error message returned by verify.  An empty
// string will be accepted as the default value even if it would fail
// verification.
func (p *Pollster) EnterVerifyDefault(valueName string, verify VerifyFunc, defVal string) (string, error) {
	var verifyDefault VerifyFunc
	if verify != nil {
		verifyDefault = func(s string) (ok bool, errmsg string, err error) {
			if s == "" {
				return true, "", nil
			}
			return verify(s)
		}
	}
	s, err := QueryVerify("Enter "+valueName+" ["+defVal+"]: ", p.scanner, p.out, p.errOut, verifyDefault)
	if err != nil {
		return "", errors.Trace(err)
	}
	if s == "" {
		return defVal, nil
	}
	return s, nil
}

// YN queries the user with a yes no question q (which should not include a
// question mark at the end).  It uses defVal as the default answer.
func (p *Pollster) YN(q string, defVal bool) (bool, error) {
	defaultStr := "(y/N)"
	if defVal {
		defaultStr = "(Y/n)"
	}
	verify := func(s string) (ok bool, errmsg string, err error) {
		_, err = yesNoConvert(s, defVal)
		if err != nil {
			return false, err.Error(), nil
		}
		return true, "", nil
	}
	a, err := QueryVerify(q+"? "+defaultStr+": ", p.scanner, p.out, p.errOut, verify)
	if err != nil {
		return false, errors.Trace(err)
	}
	return yesNoConvert(a, defVal)
}

func yesNoConvert(s string, defVal bool) (bool, error) {
	if s == "" {
		return defVal, nil
	}
	switch strings.ToLower(s) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errors.Errorf("Invalid entry: %q, please choose y or n.", s)
	}
}

// VerifyOptions is the verifier used by pollster.Select.
func VerifyOptions(singular string, options []string, hasDefault bool) VerifyFunc {
	return func(s string) (ok bool, errmsg string, err error) {
		if s == "" {
			return hasDefault, "", nil
		}
		for _, opt := range options {
			if strings.ToLower(opt) == strings.ToLower(s) {
				return true, "", nil
			}
		}
		return false, fmt.Sprintf("Invalid %s: %q", singular, s), nil
	}
}

func multiVerify(singular, plural string, options []string, hasDefault bool) VerifyFunc {
	return func(s string) (ok bool, errmsg string, err error) {
		if s == "" {
			return hasDefault, "", nil
		}
		vals := set.NewStrings(multiSplit(s)...)
		opts := set.NewStrings(options...)
		unknowns := vals.Difference(opts)
		if len(unknowns) > 1 {
			list := `"` + strings.Join(unknowns.SortedValues(), `", "`) + `"`
			return false, fmt.Sprintf("Invalid %s: %s", plural, list), nil
		}
		if len(unknowns) > 0 {
			return false, fmt.Sprintf("Invalid %s: %q", singular, unknowns.Values()[0]), nil
		}
		return true, "", nil
	}
}

func multiSplit(s string) []string {
	chosen := strings.Split(s, ",")
	for i := range chosen {
		chosen[i] = strings.TrimSpace(chosen[i])
	}
	return chosen
}

func sprint(t *template.Template, data interface{}) (string, error) {
	b := &bytes.Buffer{}
	if err := t.Execute(b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// QuerySchema takes a jsonschema and queries the user to input value(s) for the
// schema.  It returns an object as defined by the schema (generally a
// map[string]interface{} for objects, etc).
func (p *Pollster) QuerySchema(schema *jsonschema.Schema) (interface{}, error) {
	if len(schema.Type) == 0 {
		return nil, errors.Errorf("invalid schema, no type specified")
	}
	if len(schema.Type) > 1 {
		return nil, errors.Errorf("don't know how to query for a value with multiple types")
	}
	var v interface{}
	var err error
	if schema.Type[0] == jsonschema.ObjectType {
		v, err = p.queryObjectSchema(schema)
	} else {
		v, err = p.queryOneSchema(schema)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return v, nil
}

func (p *Pollster) queryObjectSchema(schema *jsonschema.Schema) (map[string]interface{}, error) {
	// TODO(natefinch): support for optional values.
	vals := map[string]interface{}{}
	if len(schema.Order) != 0 {
		m, err := p.queryOrder(schema)
		if err != nil {
			return nil, errors.Trace(err)
		}
		vals = m
	} else {
		// traverse alphabetically
		for _, name := range names(schema.Properties) {
			v, err := p.queryProp(schema.Properties[name])
			if err != nil {
				return nil, errors.Trace(err)
			}
			vals[name] = v
		}
	}

	if schema.AdditionalProperties != nil {
		if err := p.queryAdditionalProps(vals, schema); err != nil {
			return nil, errors.Trace(err)
		}
	}

	return vals, nil
}

// names returns the list of names of schema in alphabetical order.
func names(m map[string]*jsonschema.Schema) []string {
	ret := make([]string, 0, len(m))
	for n := range m {
		ret = append(ret, n)
	}
	sort.Strings(ret)
	return ret
}

func (p *Pollster) queryOrder(schema *jsonschema.Schema) (map[string]interface{}, error) {
	vals := map[string]interface{}{}
	for _, name := range schema.Order {
		prop, ok := schema.Properties[name]
		if !ok {
			return nil, errors.Errorf("property %q from Order not in schema", name)
		}
		v, err := p.queryProp(prop)
		if err != nil {
			return nil, errors.Trace(err)
		}
		vals[name] = v
	}
	return vals, nil
}

func (p *Pollster) queryProp(prop *jsonschema.Schema) (interface{}, error) {
	if isObject(prop) {
		return p.queryObjectSchema(prop)
	}
	return p.queryOneSchema(prop)
}

func (p *Pollster) queryAdditionalProps(vals map[string]interface{}, schema *jsonschema.Schema) error {
	if schema.AdditionalProperties.Type[0] != jsonschema.ObjectType {
		return errors.Errorf("don't know how to query for additional properties of type %q", schema.AdditionalProperties.Type[0])
	}

	verifyName := func(s string) (ok bool, errmsg string, err error) {
		if s == "" {
			return false, "", nil
		}
		if _, ok := vals[s]; ok {
			return false, fmt.Sprintf("%s %q already exists", strings.Title(schema.Singular), s), nil
		}
		return true, "", nil
	}

	localEnvVars := func(envVars []string) string {
		for _, envVar := range envVars {
			if value, ok := os.LookupEnv(envVar); ok && value != "" {
				return value
			}
		}
		return ""
	}

	// Currently we assume we always prompt for at least one value for
	// additional properties, but we may want to change this to ask if they want
	// to enter any at all.
	for {
		// We assume that the name of the schema is the name of the object the
		// schema describes, and for additional properties the property name
		// (i.e. map key) is the "name" of the thing.
		var name string
		var err error

		// Note: here we check that schema.Default is empty as well.
		defFromEnvVar := localEnvVars(schema.EnvVars)
		if (schema.Default == nil || schema.Default == "") && defFromEnvVar == "" {
			name, err = p.EnterVerify(schema.Singular+" name", verifyName)
			if err != nil {
				return errors.Trace(err)
			}
		} else {
			// If we set a prompt default, that'll get returned as the value,
			// but it's not the actual value that is the default, so fix that,
			// if an environment variable wasn't used.
			var def string
			if schema.PromptDefault != nil {
				def = fmt.Sprintf("%v", schema.PromptDefault)
			}
			if defFromEnvVar != "" {
				def = defFromEnvVar
			}
			if def == "" {
				def = fmt.Sprintf("%v", schema.Default)
			}

			name, err = p.EnterVerifyDefault(schema.Singular, verifyName, def)
			if err != nil {
				return errors.Trace(err)
			}

			if name == def && schema.PromptDefault != nil && name != defFromEnvVar {
				name = fmt.Sprintf("%v", schema.Default)
			}
		}

		v, err := p.queryObjectSchema(schema.AdditionalProperties)
		if err != nil {
			return errors.Trace(err)
		}
		vals[name] = v
		more, err := p.YN("Enter another "+schema.Singular, false)
		if err != nil {
			return errors.Trace(err)
		}
		if !more {
			break
		}
	}

	return nil
}

func (p *Pollster) queryOneSchema(schema *jsonschema.Schema) (interface{}, error) {
	if len(schema.Type) == 0 {
		return nil, errors.Errorf("invalid schema, no type specified")
	}
	if len(schema.Type) > 1 {
		return nil, errors.Errorf("don't know how to query for a value with multiple types")
	}
	if len(schema.Enum) == 1 {
		// if there's only one possible value, don't bother prompting, just
		// return that value.
		return schema.Enum[0], nil
	}
	if schema.Type[0] == jsonschema.ArrayType {
		return p.queryArray(schema)
	}
	if len(schema.Enum) > 1 {
		return p.selectOne(schema)
	}
	var verify VerifyFunc
	switch schema.Format {
	case "":
		// verify stays nil
	case jsonschema.FormatURI:
		if p.VerifyURLs != nil {
			verify = p.VerifyURLs
		} else {
			verify = uriVerify
		}
	case FormatCertFilename:
		if p.VerifyCertFile != nil {
			verify = p.VerifyCertFile
		}
	default:
		// TODO(natefinch): support more formats
		return nil, errors.Errorf("unsupported format type: %q", schema.Format)
	}

	if schema.Default == nil {
		a, err := p.EnterVerify(schema.Singular, verify)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return convert(a, schema.Type[0])
	}

	var def string
	if schema.PromptDefault != nil {
		def = fmt.Sprintf("%v", schema.PromptDefault)
	}
	var defFromEnvVar string
	if len(schema.EnvVars) > 0 {
		for _, envVar := range schema.EnvVars {
			value := os.Getenv(envVar)
			if value != "" {
				defFromEnvVar = value
				def = defFromEnvVar
				break
			}
		}
	}
	if def == "" {
		def = fmt.Sprintf("%v", schema.Default)
	}

	a, err := p.EnterVerifyDefault(schema.Singular, verify, def)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If we set a prompt default, that'll get returned as the value,
	// but it's not the actual value that is the default, so fix that,
	// if an environment variable wasn't used.
	if a == def && schema.PromptDefault != nil && a != defFromEnvVar {
		a = fmt.Sprintf("%v", schema.Default)
	}

	return convert(a, schema.Type[0])
}

func (p *Pollster) queryArray(schema *jsonschema.Schema) (interface{}, error) {
	if !supportedArraySchema(schema) {
		b, err := schema.MarshalJSON()
		if err != nil {
			// shouldn't ever happen
			return nil, errors.Errorf("unsupported schema for an array")
		}
		return nil, errors.Errorf("unsupported schema for an array: %s", b)
	}
	var def string
	if schema.Default != nil {
		def = schema.Default.(string)
	}
	if schema.PromptDefault != nil {
		def = schema.PromptDefault.(string)
	}
	var array []string
	if def != "" {
		array = []string{def}
	}
	return p.MultiSelect(MultiList{
		Singular: schema.Singular,
		Plural:   schema.Plural,
		Options:  optFromEnum(schema.Items.Schemas[0]),
		Default:  array,
	})
}

func uriVerify(s string) (ok bool, errMsg string, err error) {
	if s == "" {
		return false, "", nil
	}
	_, err = url.Parse(s)
	if err != nil {
		return false, fmt.Sprintf("Invalid URI: %q", s), nil
	}
	return true, "", nil
}

func supportedArraySchema(schema *jsonschema.Schema) bool {
	// TODO(natefinch): support arrays without schemas.
	// TODO(natefinch): support arrays with multiple schemas.
	// TODO(natefinch): support arrays without Enums.
	if schema.Items == nil ||
		len(schema.Items.Schemas) != 1 ||
		len(schema.Items.Schemas[0].Enum) == 0 ||
		len(schema.Items.Schemas[0].Type) != 1 {
		return false
	}
	switch schema.Items.Schemas[0].Type[0] {
	case jsonschema.IntegerType,
		jsonschema.StringType,
		jsonschema.BooleanType,
		jsonschema.NumberType:
		return true
	default:
		return false
	}
}

func optFromEnum(schema *jsonschema.Schema) []string {
	ret := make([]string, len(schema.Enum))
	for i := range schema.Enum {
		ret[i] = fmt.Sprint(schema.Enum[i])
	}
	return ret
}

func (p *Pollster) selectOne(schema *jsonschema.Schema) (interface{}, error) {
	options := make([]string, len(schema.Enum))
	for i := range schema.Enum {
		options[i] = fmt.Sprint(schema.Enum[i])
	}
	def := ""
	if schema.Default != nil {
		def = fmt.Sprint(schema.Default)
	}
	a, err := p.Select(List{
		Singular: schema.Singular,
		Plural:   schema.Plural,
		Options:  options,
		Default:  def,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	if schema.Default != nil && a == "" {
		return schema.Default, nil
	}
	return convert(a, schema.Type[0])
}

func isObject(schema *jsonschema.Schema) bool {
	for _, t := range schema.Type {
		if t == jsonschema.ObjectType {
			return true
		}
	}
	return false
}

// convert converts the given string to a specific value based on the schema
// type that validated it.
func convert(s string, t jsonschema.Type) (interface{}, error) {
	switch t {
	case jsonschema.IntegerType:
		return strconv.Atoi(s)
	case jsonschema.NumberType:
		return strconv.ParseFloat(s, 64)
	case jsonschema.StringType:
		return s, nil
	case jsonschema.BooleanType:
		switch strings.ToLower(s) {
		case "y", "yes", "true", "t":
			return true, nil
		case "n", "no", "false", "f":
			return false, nil
		default:
			return nil, errors.Errorf("unknown value for boolean type: %q", s)
		}
	default:
		return nil, errors.Errorf("don't know how to convert value %q of type %q", s, t)
	}
}

// byteAtATimeReader causes all reads to return a single byte.  This prevents
// things line bufio.scanner from reading past the end of a line, which can
// cause problems when we do wacky things like reading directly from the
// terminal for password style prompts.
type byteAtATimeReader struct {
	io.Reader
}

func (r byteAtATimeReader) Read(out []byte) (int, error) {
	return r.Reader.Read(out[:1])
}
