package dlv

import (
	"fmt"
	"maps"
)

// Config is a type that represents a map of configuration options where the key is a string and the value can be any type.
// They are passed to Delve as command line option.
type Config map[string]any

// Option describes a functional option for configuring a Config instance.
type Option func(*Config)

// WithDefault returns an Option that sets default configuration values for a Config instance.
func WithDefault() Option {
	return func(o *Config) {
		o.apply(
			Headless(),
			WaitDebugger(),
			WithApiVersion(2),
			WithPort(0))
	}
}

func (o *Config) apply(opts ...Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// args constructs a slice of command line argument strings from the Config map.
func (o *Config) args() []string {
	args := make([]string, 0, len(*o))
	for k, v := range *o {
		switch val := v.(type) {
		case bool:
			if val {
				args = append(args, fmt.Sprintf("--%s", k))
			}
		default:
			if v != nil {
				args = append(args, fmt.Sprintf("--%s=%v", k, v))
			}
		}
	}
	return args
}

// With sets a key-value pair in the Config struct. Handles zero, one, or multiple values for a given key.
func With(key string, value ...any) Option {
	return func(o *Config) {
		switch len(value) {
		case 0:
			 (*o)[key] = true
		case 1:
			(*o)[key] =  value[0]
		default:
			(*o)[key] = value
		}
	}
}

// WithPort configures the listening port for the application by setting the "listen" option to the specified port.
func WithPort(port int) Option {
	return With("listen", fmt.Sprintf(":%d", port))
}

// WithApiVersion sets the API version in the options with the specified version number.
func WithApiVersion(version int) Option {
	return With("api-version", version)
}

// Headless sets the "headless" option to true, which runs delve in server mode
func Headless() Option {
	return With("headless", true)
}

// WaitDebugger set the "continue" option to false,
// the debugged application will wait until a debugger is attached, after having generating a first
// log indicating on which endpoint it listens.
func WaitDebugger() Option {
	return With("continue",false)
}

// NoWait set the debugger to continue execution immediately. Debugged application
// wouldn't wait for a debugger to be attached.
func NoWait() Option {
	return func(o *Config) {
		maps.Insert(*o, maps.All( Config{
		"continue":           true,
		"accept-multiclient": true,
	}))
	}
}
