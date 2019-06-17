// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/generate/schemagen/gen"
)

func main() {
	// the first argument here will be the name of the binary, so we ignore
	// argument 0 when looking for the filepath.
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Expected one argument: filepath of json schema to save.")
		os.Exit(1)
	}

	result, err := gen.Generate(apiServerShim{})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	jsonSchema, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(os.Args[1], jsonSchema, 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type apiServerShim struct{}

func (apiServerShim) AllFacades() gen.Registry {
	return apiserver.AllFacades()
}
