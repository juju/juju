// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"text/template"
)

// renderable is a generic interface for types that implement fmt.Stringer.
// It provides a method Subs which returns a slice of items.
type renderable[HeaderData any, SubData fmt.Stringer] interface {
	HeaderData() HeaderData
	SubDatas() []SubData
}

// newRenderer creates a renderer function to generate files from templates.
// headerTemplate is the template for the file header content.
// subTemplate is the template for the repeated sections in the file.
// The returned function takes outputFile and params to generate the output.
func newRenderer[HeaderData any, SubData fmt.Stringer](
	headerTemplate,
	subTemplate string,
) func(outputFile string, params renderable[HeaderData, SubData]) {
	return func(outputFile string, params renderable[HeaderData, SubData]) {
		// Create the output file
		file, err := os.Create(outputFile)
		if err != nil {
			fmt.Printf("Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()

		// Write the file header
		headerTmpl, err := template.New("header").Parse(headerTemplate)
		if err != nil {
			fmt.Printf("Error parsing header template: %v\n", err)
			os.Exit(1)
		}

		if err := headerTmpl.Execute(file, params); err != nil {
			fmt.Printf("Error executing header template: %v\n", err)
			os.Exit(1)
		}

		// Parse the type template once
		subTmpl, err := template.New("type").Parse(subTemplate)
		if err != nil {
			fmt.Printf("Error parsing type template: %v\n", err)
			os.Exit(1)
		}

		// Write each type to the file
		subs := params.SubDatas()
		for _, uuidType := range subs {
			if err := subTmpl.Execute(file, uuidType); err != nil {
				fmt.Printf("Error executing type template for %s: %v\n",
					uuidType.String(), err)
				os.Exit(1)
			}
		}

		fmt.Printf("Generated %s with %d UUID types\n", outputFile, len(subs))
	}
}
