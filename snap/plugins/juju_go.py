# -*- Mode:Python; indent-tabs-mode:nil; tab-width:4 -*-
#
# Copyright (C) 2018 Canonical Ltd
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3 as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

"""The juju-go plugin used for snapping juju.

This plugin uses the common plugin keywords as well as those for "sources".
For more information check the 'plugins' topic for the former and the
'sources' topic for the latter.

Additionally, this plugin uses the following plugin-specific keywords:

    - go-packages:
      (list of strings)
      Go packages to build/install, these must be a "main" package.
      Dependencies should have already been retrieved.
      Packages that are not "main" will not cause an error, but would
      not be useful either.
"""

import logging
import os
import shutil

import snapcraft
from snapcraft import common
from snapcraft import file_utils

logger = logging.getLogger(__name__)

# TODO(hpidcock): move to upstream go plugin when it has the features we need.
class JujuGoPlugin(snapcraft.BasePlugin):
    @classmethod
    def schema(cls):
        schema = super().schema()
        schema["properties"]["go-channel"] = {
            "type": "string",
            "default": "latest/stable",
        }
        schema["properties"]["go-packages"] = {
            "type": "array",
            "minitems": 1,
            "uniqueItems": True,
            "items": {"type": "string"},
            "default": [],
        }
        schema["properties"]["go-external-strings"] = {
            "type": "object",
            "additionalProperties": {"type": "string"},
            "default": {},
        }
        schema["properties"]["go-static"] = {
            "type": "boolean",
            "default": False,
        }
        return schema

    @classmethod
    def get_build_properties(cls):
        # Inform Snapcraft of the properties associated with building. If these
        # change in the YAML Snapcraft will consider the build step dirty.
        return ["go-packages", "go-external-strings", "go-channel", "go-static"]

    def __init__(self, name, options, project):
        super().__init__(name, options, project)
        self.build_packages.extend(["git"])
        self.build_snaps.extend(["go/"+self.options.go_channel])
        self._gopath = os.path.join(self.partdir, "go")
        self._gopath_src = os.path.join(self._gopath, "src")
        self._gopath_bin = os.path.join(self._gopath, "bin")
        self._gopath_pkg = os.path.join(self._gopath, "pkg")

    def pull(self):
        super().pull()

        logger.info("Obtaining project dependencies...")
        self._run(
            [
                "go",
                "mod",
                "vendor",
            ]
        )

    def clean_pull(self):
        super().clean_pull()

        # Remove the gopath (if present)
        if os.path.exists(self._gopath):
            shutil.rmtree(self._gopath)

    def build(self):
        super().build()

        cmd = ["go", "install", "-mod=vendor"]
        cmd.append("-ldflags")
        flags = ""
        if self.options.go_static:
            flags = '-extldflags "-static"'

        if len(self.options.go_external_strings) > 0:
            for k, v in self.options.go_external_strings.items():
                flags += " -X {}={}".format(k, v)
        cmd.append(flags)

        for go_package in self.options.go_packages:
            cmd.append(go_package)

        self._run(cmd)

        install_bin_path = os.path.join(self.installdir, "bin")
        os.makedirs(install_bin_path, exist_ok=True)
        os.makedirs(self._gopath_bin, exist_ok=True)
        for binary in os.listdir(self._gopath_bin):
            binary_path = os.path.join(self._gopath_bin, binary)
            shutil.copy2(binary_path, install_bin_path)

    def clean_build(self):
        super().clean_build()

        if os.path.isdir(self._gopath_bin):
            shutil.rmtree(self._gopath_bin)

        if os.path.isdir(self._gopath_pkg):
            shutil.rmtree(self._gopath_pkg)

    def _run(self, cmd, **kwargs):
        env = self._build_environment()
        return self.run(cmd, cwd=self.sourcedir, env=env, **kwargs)

    def _build_environment(self):
        env = os.environ.copy()
        env["GOPATH"] = self._gopath
        env["GOBIN"] = self._gopath_bin

        if self.options.go_static:
            env["CGO_ENABLED"] = "0"
        else:
            include_paths = []
            for root in [self.installdir, self.project.stage_dir]:
                include_paths.extend(
                    common.get_library_paths(root, self.project.arch_triplet)
                )
            flags = common.combine_paths(include_paths, "-L", " ")
            env["CGO_LDFLAGS"] = "{} {} {}".format(
                env.get("CGO_LDFLAGS", ""), flags, env.get("LDFLAGS", "")
            )

        return env
