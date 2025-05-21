// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 4 || (len(os.Args)-3)%2 != 0 {
		fmt.Println("Usage: go run generate/uuidgen <output-file> <package-name> <type-name1> <description1> [<type-name2> <description2> ...]")
		fmt.Println("Example: go run generate/uuidgen uuid.go relation UUID \"relation\" UnitUUID \"relation unit\"")
		os.Exit(1)
	}

	fmt.Println(os.Args)

	outputFile := os.Args[1]
	packageName := os.Args[2]

	// Parse type/description pairs
	var types []UUIDType
	for i := 3; i < len(os.Args); i += 2 {
		if i+1 < len(os.Args) {
			types = append(types, UUIDType{
				TypeName:    os.Args[i],
				Description: os.Args[i+1],
			})
		}
	}

	params := FileParams{
		Package: packageName,
		types:   types,
	}

	generateTypeFile(outputFile, params)
	generateTestGenFile(outputFile, params)
	generateTestTypeFile(outputFile, params)
}
