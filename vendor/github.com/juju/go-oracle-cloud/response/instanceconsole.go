// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// InstanceConsole you can use the console output of an
// instance to diagnose failures that occurred while booting
// the instance. The instanceconsole object is created when
// an instance is launched, and it is destroyed
// when the instance is deleted.
type InstanceConsole struct {

	// Timestamp is the time when the console output was created.
	Timestamp string `json:"timestamp"`

	// Output is the serial console output of the instance.
	Output string `json:"output"`

	// Uri is the Uniform Resource Identifier
	Uri string `json:"uri"`

	// Name is the name of the instance for
	// which the console output is displayed.
	Name string `json:"name"`
}
