/*
Copyright (c) 2014 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package importx

import (
	"flag"
	"path"
	"strings"

	"github.com/juju/govmomi/govc/cli"
)

type ova struct {
	*ovf
}

func init() {
	cli.Register("import.ova", &ova{&ovf{}})
}

func (cmd *ova) Usage() string {
	return "PATH_TO_OVA"
}

func (cmd *ova) Run(f *flag.FlagSet) error {
	file, err := cmd.Prepare(f)

	if err != nil {
		return err
	}

	cmd.Archive = &TapeArchive{file}

	return cmd.Import(file)
}

func (cmd *ova) Import(fpath string) error {
	// basename i | sed -e s/\.ova$/*.ovf/
	ovf := strings.TrimSuffix(path.Base(fpath), path.Ext(fpath)) + "*.ovf"

	return cmd.ovf.Import(ovf)
}
