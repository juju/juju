// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"

	"github.com/juju/errors"
)

var fileTemplate = `
// Copyright {{.CopyrightYear}} Canonical Ltd.
// Copyright {{.CopyrightYear}} Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package {{.Pkgname}}

// Generated code - do not edit.

const addJujuUser = {{.AddJujuUser}}
const windowsPowershellHelpers = {{.WindowsPowershellHelper}}
`[1:]

// compoundWinPowershellFunc returns the windows powershell helper
// functions declared under the windowsuserdatafiles/ dir
func compoundWinPowershellFuncs() (string, error) {
	var winPowershellFunc bytes.Buffer

	filenames := []string{
		"windowsuserdatafiles/retry.ps1",
		"windowsuserdatafiles/untar.ps1",
		"windowsuserdatafiles/filesha256.ps1",
		"windowsuserdatafiles/invokewebrequest.ps1",
	}
	for _, filename := range filenames {
		content, err := ioutil.ReadFile(filename)
		if err != nil {
			return "", errors.Trace(err)
		}
		winPowershellFunc.Write(content)
	}

	return winPowershellFunc.String(), nil
}

// CompoundJujuUser returns the windows powershell funcs with c# bindings
// declared under the windowsuserdatafiles/ dir
func compoundJujuUser() (string, error) {

	// note that addJujuUser.ps1 has hinting format locations for sprintf
	// in that hinting locations we will add the c# scripts under the same file
	content, err := ioutil.ReadFile("windowsuserdatafiles/addJujuUser.ps1")
	if err != nil {
		return "", errors.Trace(err)
	}

	// take the two addJujuUser data c# scripts and construct the powershell
	// script for adding user for juju in windows
	cryptoAPICode, err := ioutil.ReadFile("windowsuserdatafiles/CryptoApi.cs")
	if err != nil {
		return "", errors.Trace(err)
	}
	carbonCode, err := ioutil.ReadFile("windowsuserdatafiles/Carbon.cs")
	if err != nil {
		return "", errors.Trace(err)
	}

	return fmt.Sprintf(string(content), string(cryptoAPICode), string(carbonCode)), nil
}

// This generator reads from a file and generates a Go file with the
// file's content assigned to a go constant.
func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: winuserdata <copyrightyear> <gofile> <pkgname>")
	}

	addJujuUser, err := compoundJujuUser()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	winpowershell, err := compoundWinPowershellFuncs()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	type content struct {
		AddJujuUser             string
		WindowsPowershellHelper string
		CopyrightYear           string
		Pkgname                 string
	}

	t, err := template.New("").Parse(fileTemplate)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	addJujuUser = fmt.Sprintf("\n%s", addJujuUser)
	winpowershell = fmt.Sprintf("\n%s", winpowershell)

	// Quote any ` in the data.
	addJujuUser = strings.Replace(addJujuUser, "`", "` + \"`\" + `", -1)
	winpowershell = strings.Replace(winpowershell, "`", "` + \"`\" + `", -1)

	_ = t.Execute(&buf, content{
		AddJujuUser:             fmt.Sprintf("`%s`", addJujuUser),
		WindowsPowershellHelper: fmt.Sprintf("`%s`", winpowershell),
		Pkgname:                 os.Args[3],
		CopyrightYear:           os.Args[1],
	})

	err = ioutil.WriteFile(os.Args[2], buf.Bytes(), 0644)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
