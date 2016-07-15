// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/environschema.v1/form"
)

var showDescriptions = flag.Bool("v", false, "show descriptions")

func main() {
	flag.Parse()

	f := form.IOFiller{
		ShowDescriptions: *showDescriptions,
	}
	fmt.Println(`formtest:
This is a simple interactive test program for environschema forms.
Expect the prompts to be as follows:

e-mail [user@example.com]:
name:
password: 
PIN [****]: 

The entered values will be displayed at the end.
`)
	os.Setenv("PIN", "1234")
	os.Setenv("EMAIL", "user@example.com")
	r, err := f.Fill(form.Form{
		Title: "Test Form",
		Fields: environschema.Fields{
			"name": environschema.Attr{
				Description: "Your full name.",
				Type:        environschema.Tstring,
				Mandatory:   true,
			},
			"email": environschema.Attr{
				Description: "Your email address.",
				Type:        environschema.Tstring,
				EnvVar:      "EMAIL",
			},
			"password": environschema.Attr{
				Description: "Your very secret password.",
				Type:        environschema.Tstring,
				Secret:      true,
				Mandatory:   true,
			},
			"pin": environschema.Attr{
				Description: "Some PIN that you have probably forgotten.",
				Type:        environschema.Tint,
				EnvVar:      "PIN",
				Secret:      true,
			},
		}})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	b, err := json.MarshalIndent(r, "", "\t")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}
