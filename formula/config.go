package formula

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/ensemble/go/schema"
	"launchpad.net/goyaml"
	"os"
	"strconv"
)

// Option represents a single configuration option that is declared
// as supported by a formula in its config.yaml file.
type Option struct {
	Title       string
	Description string
	Type        string
	Default     interface{}
}

// Config represents the supported configuration options for a formula,
// as declared in its config.yaml file.
type Config struct {
	Options map[string]Option
}

// ReadConfig reads a config.yaml file and returns its representation.
func ReadConfig(path string) (config *Config, err os.Error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	config, err = ParseConfig(data)
	if err != nil {
		err = os.NewError(fmt.Sprintf("%s: %s", path, err))
	}
	return
}

// ParseConfig parses the content of a config.yaml file and returns
// its representation.
func ParseConfig(data []byte) (config *Config, err os.Error) {
	raw := make(map[interface{}]interface{})
	err = goyaml.Unmarshal(data, raw)
	if err != nil {
		return
	}
	v, err := configSchema.Coerce(raw, nil)
	if err != nil {
		return
	}
	config = &Config{}
	config.Options = make(map[string]Option)
	m := v.(schema.MapType)
	for name, infov := range m["options"].(schema.MapType) {
		opt := infov.(schema.MapType)
		optTitle, _ := opt["title"].(string)
		optType, _ := opt["type"].(string)
		optDescr, _ := opt["description"].(string)
		optDefault, _ := opt["default"]
		config.Options[name.(string)] = Option{
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
func (c *Config) Validate(values map[string]string) (processed map[string]interface{}, err os.Error) {
	out := make(map[string]interface{})
	for k, v := range values {
		opt, ok := c.Options[k]
		if !ok {
			return nil, os.NewError(fmt.Sprintf("Unknown configuration option: %q", k))
		}
		switch opt.Type {
		case "string":
			out[k] = v
		case "int":
			i, err := strconv.Atoi64(v)
			if err != nil {
				return nil, os.NewError(fmt.Sprintf("Value for %q is not an int: %q", k, v))
			}
			out[k] = i
		case "float":
			f, err := strconv.Atof64(v)
			if err != nil {
				return nil, os.NewError(fmt.Sprintf("Value for %q is not a float: %q", k, v))
			}
			out[k] = f
		default:
			panic(fmt.Sprintf("Internal error: option type %q is unknown to Validate", opt.Type))
		}
	}
	for k, opt := range c.Options {
		if _, ok := out[k]; !ok && opt.Default != nil {
			out[k] = opt.Default
		}
	}
	return out, nil
}

var optionSchema = schema.FieldMap(
	schema.Fields{
		"type":        schema.OneOf(schema.Const("string"), schema.Const("int"), schema.Const("float")),
		"default":     schema.OneOf(schema.String(), schema.Int(), schema.Float()),
		"description": schema.String(),
	},
	schema.Optional{"default", "description"},
)

var configSchema = schema.FieldMap(
	schema.Fields{
		"options": schema.Map(schema.String(), optionSchema),
	},
	nil,
)
