package charm

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/schema"
	"reflect"
	"strconv"
)

// Option represents a single configuration option that is declared
// as supported by a charm in its config.yaml file.
type Option struct {
	Title       string
	Description string
	Type        string
	Default     interface{}
}

// Config represents the supported configuration options for a charm,
// as declared in its config.yaml file.
type Config struct {
	Options map[string]Option
}

// NewConfig returns a new Config without any options.
func NewConfig() *Config {
	return &Config{make(map[string]Option)}
}

// ReadConfig reads a config.yaml file and returns its representation.
func ReadConfig(r io.Reader) (config *Config, err error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	raw := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, raw)
	if err != nil {
		return
	}
	v, err := configSchema.Coerce(raw, nil)
	if err != nil {
		return nil, errors.New("config: " + err.Error())
	}
	config = NewConfig()
	m := v.(map[string]interface{})
	for name, infov := range m["options"].(map[string]interface{}) {
		opt := infov.(map[string]interface{})
		optTitle, _ := opt["title"].(string)
		optType, _ := opt["type"].(string)
		optDescr, _ := opt["description"].(string)
		optDefault := opt["default"]
		if optDefault != nil {
			if reflect.TypeOf(optDefault).Kind() != validTypes[optType] {
				msg := "Bad default for %q: %v is not of type %s"
				return nil, fmt.Errorf(msg, name, optDefault, optType)
			}
		}
		config.Options[name] = Option{
			Title:       optTitle,
			Type:        optType,
			Description: optDescr,
			Default:     optDefault,
		}
	}
	return
}

// Validate processes the values in the input map according to the
// configuration in config, doing the following operations:
//
// - Values are converted from strings to the types defined
// - Options with default values are introduced for missing keys
// - Unknown keys and badly typed values are reported as errors
// 
func (c *Config) Validate(values map[string]string) (processed map[string]interface{}, err error) {
	out := make(map[string]interface{})
	for k, v := range values {
		opt, ok := c.Options[k]
		if !ok {
			return nil, fmt.Errorf("Unknown configuration option: %q", k)
		}
		switch opt.Type {
		case "string":
			out[k] = v
		case "int":
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Value for %q is not an int: %q", k, v)
			}
			out[k] = i
		case "float":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("Value for %q is not a float: %q", k, v)
			}
			out[k] = f
		case "boolean":
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("Value for %q is not a boolean: %q", k, v)
			}
			out[k] = b
		default:
			panic(fmt.Errorf("Internal error: option type %q is unknown to Validate", opt.Type))
		}
	}
	for k, opt := range c.Options {
		if _, ok := out[k]; !ok && opt.Default != nil {
			out[k] = opt.Default
		}
	}
	return out, nil
}

var validTypes = map[string]reflect.Kind{
	"string":  reflect.String,
	"int":     reflect.Int64,
	"boolean": reflect.Bool,
	"float":   reflect.Float64,
}

var optionSchema = schema.FieldMap(
	schema.Fields{
		"type":        schema.OneOf(schema.Const("string"), schema.Const("int"), schema.Const("float"), schema.Const("boolean")),
		"default":     schema.OneOf(schema.String(), schema.Int(), schema.Float(), schema.Bool()),
		"description": schema.String(),
	},
	schema.Optional{"default", "description"},
)

var configSchema = schema.FieldMap(
	schema.Fields{
		"options": schema.StringMap(optionSchema),
	},
	nil,
)
