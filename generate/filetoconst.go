// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
)

var fileTemplate = `
// Copyright {{.CopyrightYear}} Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// Generated code - do not edit.

const {{.ConstName}} = {{.Content}}
`[1:]

// This generator reads from a file and generates a Go file with the
// file's content assigned to a go constant.
func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: filetoconst <constname> <infile> <gofile> <copyrightyear>")
	}
	data, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	t, err := template.New("").Parse(fileTemplate)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	var buf bytes.Buffer
	type content struct {
		ConstName     string
		Content       string
		CopyrightYear string
	}
	contextData := fmt.Sprintf("\n%s", string(data))
	// Quote any ` in the data.
	contextData = strings.Replace(contextData, "`", "`+\"`\"+`", -1)
	t.Execute(&buf, content{
		ConstName:     os.Args[1],
		Content:       fmt.Sprintf("`%s`", contextData),
		CopyrightYear: os.Args[4],
	})

	err = ioutil.WriteFile(os.Args[3], buf.Bytes(), 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
