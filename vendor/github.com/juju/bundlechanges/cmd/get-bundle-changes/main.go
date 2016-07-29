// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/bundlechanges"
)

func main() {
	flag.Usage = usage
	flag.Parse()
	if len(flag.Args()) > 1 {
		fmt.Fprintln(os.Stderr, "need a bundle path as first and only argument")
		os.Exit(2)
	}
	r := os.Stdin
	if path := flag.Arg(0); path != "" {
		var err error
		if r, err = os.Open(path); err != nil {
			fmt.Fprintf(os.Stderr, "invalid bundle path: %s\n", err)
			os.Exit(2)
		}
		defer r.Close()
	}
	if err := process(r, os.Stdout); err != nil {
		if verr, ok := err.(*charm.VerificationError); ok {
			fmt.Fprintf(os.Stderr, "the given bundle is not valid:\n")
			for _, err := range verr.Errors {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "unable to parse bundle: %s\n", err)
		}
		os.Exit(1)
	}
}

// usage outputs instructions on how to use this command.
func usage() {
	fmt.Fprintln(os.Stderr, "usage: get-bundle-changes [bundle]")
	fmt.Fprintln(os.Stderr, "bundle can also be provided on stdin")
	flag.PrintDefaults()
	os.Exit(2)
}

// process generates and print to w the set of changes required to deploy
// the bundle data to be retrieved using r.
func process(r io.Reader, w io.Writer) error {
	// Read the bundle data.
	data, err := charm.ReadBundleData(r)
	if err != nil {
		return err
	}
	// Validate the bundle.
	if err := data.Verify(nil, nil); err != nil {
		return err
	}
	// Generate the changes and convert them to the standard form.
	changes := bundlechanges.FromData(data)
	records := make([]*record, len(changes))
	for i, change := range changes {
		records[i] = &record{
			Id:       change.Id(),
			Requires: change.Requires(),
			Method:   change.Method(),
			Args:     change.GUIArgs(),
		}
	}
	// Serialize and print the records.
	content, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(content))
	return nil
}

// record holds the JSON representation of a change.
type record struct {
	// Id is the unique identifier for this change.
	Id string `json:"id"`
	// Method is the action to be performed to apply this change.
	Method string `json:"method"`
	// Args holds a list of arguments to pass to the method.
	Args []interface{} `json:"args"`
	// Requires holds a list of dependencies for this change. Each dependency
	// is represented by the corresponding change id, and must be applied
	// before this change is applied.
	Requires []string `json:"requires"`
}
