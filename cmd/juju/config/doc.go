// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package config provides a common base struct to be used for Juju's various
config commands (config, model-config, controller-config, model-defaults).
Using this struct ([ConfigCommandBase]) ensures that these commands share a
similar interface/syntax, reducing user confusion.

[ConfigCommandBase] defines a generic [Init] function which parses command-line
arguments according to this shared interface. This creates a slice
Actions []Action which the child command should use in its Run method.
The [validateActions] method encapsulates the rules regarding which actions can
be done simultaneously.

This package also defines a helper method [ReadFile], which child commands can
use to process yaml config files into an Attrs object.
*/
package config
