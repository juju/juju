// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"text/template"
)

var fileTemplate = `
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// Generated code - do not edit.

// fallbackPublicCloudInfo is the last resort public
// cloud info to use if none other is found.
const fallbackPublicCloudInfo = {{.Content}}
`[1:]

// This generator reads public cloud YAML and generates a file with that YAML
// assigned to a go constant.
func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: generatepubliccloudyaml <inyaml> <outgo>")
	}
	data, err := ioutil.ReadFile(os.Args[1])
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
		Content string
	}
	t.Execute(&buf, content{fmt.Sprintf("`\n%s`", string(data))})

	err = ioutil.WriteFile(os.Args[2], buf.Bytes(), 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
