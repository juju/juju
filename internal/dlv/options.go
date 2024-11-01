// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dlv

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"
)

// Config is a type that represents a map of configuration options where the key is a string and the value can be any type.
// They are passed to Delve as command line option.
type Config struct {
	// definedLogger allows injecting a custom logger implementation to handle logging for the wrapped program.
	definedLogger logger

	// dlvArgs contains a map of command line arguments for the Delve debugger, which would typically be added to the
	// command in the form `--[key]=[value]`
	dlvArgs map[string]any

	// sidecars are a set of function, which will be run as goroutine around the dlv execution. They are designed for
	// do some task that cannot be done before the dlv execution or need to be done concurrently (like fixing socket
	// permission :) )
	sidecars []func() error
}

// Option describes a functional option for configuring a Config instance.
type Option func(*Config)

// With sets a key-value pair in the Config struct. Handles zero, one, or multiple values for a given key.
func With(key string, value ...any) Option {
	return func(o *Config) {
		if o.dlvArgs == nil {
			o.dlvArgs = make(map[string]any)
		}
		switch len(value) {
		case 0:
			o.dlvArgs[key] = true
		case 1:
			o.dlvArgs[key] = value[0]
		default:
			o.dlvArgs[key] = value
		}
	}
}

// LoggerFunc is a type that defines a function signature for logging messages formatted according to a format specifier.
type LoggerFunc func(format string, v ...interface{})

// WithLoggerFunc allows setting a custom LoggerFunc for logging.
// It returns an Option that configures a Config instance to use the provided LoggerFunc for logging.
func WithLoggerFunc(logger LoggerFunc) Option {
	return func(o *Config) {
		o.definedLogger = loggerWrapper{logf: logger}
	}
}

// WithSidecar adds a function to be executed as a sidecar process in the Config instance.
func WithSidecar(sidecar func() error) Option {
	return func(o *Config) {
		o.sidecars = append(o.sidecars, sidecar)
	}
}

// WithPort configures the listening port for the application by setting the "listen" option to the specified port.
func WithPort(port int) Option {
	return With("listen", fmt.Sprintf(":%d", port))
}

// WithSocket configures a unix socket address to connect to the .
func WithSocket(socket string) Option {
	withSidecar := WithSidecar(func() error {
		return tryFixPermission(socket)
	})
	withSocket := With("listen", fmt.Sprintf("unix:%s", socket))

	return func(o *Config) {
		o.apply(withSidecar, withSocket)
	}
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
	return With("continue", false)
}

// NoWait set the debugger to continue execution immediately. Debugged application
// wouldn't wait for a debugger to be attached.
func NoWait() Option {
	return func(o *Config) {
		maps.Insert(o.dlvArgs, maps.All(map[string]any{
			"continue":           true,
			"accept-multiclient": true,
		}))
	}
}

// runSidecars starts each function in the sidecars slice as a separate goroutine.
func (o *Config) runSidecars() {
	for _, sidecar := range o.sidecars {
		go func() { _ = sidecar() }()
	}
}

// tryFixPermission attempts to change the file permissions of the specified path using exponential backoff retries.
func tryFixPermission(path string) error {
	return retry.Call(retry.CallArgs{
		Func: func() error { return os.Chmod(path, 0777) },
		IsFatalError: func(err error) bool {
			return !errors.Is(err, os.ErrNotExist)
		},
		Attempts:    10,
		Delay:       200 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		MaxDuration: 10 * time.Second,
		BackoffFunc: func(delay time.Duration, attempt int) time.Duration {
			return delay * time.Duration(attempt)
		},
		Clock: clock.WallClock,
	})
}

// logger returns the logger implementation to use, defaulting to the predefined defaultLogger if none is specified.
func (config *Config) logger() logger {
	if config.definedLogger == nil {
		return defaultLogger
	}
	return config.definedLogger
}

// loggerWrapper wraps a LoggerFunc to conform with the logger interface.
type loggerWrapper struct {
	logf LoggerFunc
}

// Printf logs a formatted message using a specified format and variadic arguments.
func (l loggerWrapper) Printf(format string, v ...interface{}) {
	l.logf(format, v...)
}

// defaultLogger is the default logger implementing the logger interface, using fmt.Printf to log formatted messages.
var defaultLogger = loggerWrapper{func(format string, v ...interface{}) {
	fmt.Printf(format, v...)
	fmt.Println()
}}

// apply sets multiple configuration options on a Config instance.
func (o *Config) apply(opts ...Option) {
	for _, opt := range opts {
		opt(o)
	}
}

// args constructs a slice of command line argument strings from the Config map.
func (o *Config) args() []string {
	args := make([]string, 0, len(o.dlvArgs))
	for k, v := range o.dlvArgs {
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
