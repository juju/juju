package formula

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/ensemble/go/schema"
	"launchpad.net/goyaml"
	"os"
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
