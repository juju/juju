#!/usr/bin/env python

# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import sys
import subprocess
from charmhelpers.core import hookenv


def default_execd_dir():
    return os.path.join(os.environ['CHARM_DIR'], 'exec.d')


def execd_module_paths(execd_dir=None):
    """Generate a list of full paths to modules within execd_dir."""
    if not execd_dir:
        execd_dir = default_execd_dir()

    if not os.path.exists(execd_dir):
        return

    for subpath in os.listdir(execd_dir):
        module = os.path.join(execd_dir, subpath)
        if os.path.isdir(module):
            yield module


def execd_submodule_paths(command, execd_dir=None):
    """Generate a list of full paths to the specified command within exec_dir.
    """
    for module_path in execd_module_paths(execd_dir):
        path = os.path.join(module_path, command)
        if os.access(path, os.X_OK) and os.path.isfile(path):
            yield path


def execd_run(command, execd_dir=None, die_on_error=True, stderr=subprocess.STDOUT):
    """Run command for each module within execd_dir which defines it."""
    for submodule_path in execd_submodule_paths(command, execd_dir):
        try:
            subprocess.check_output(submodule_path, stderr=stderr,
                                    universal_newlines=True)
        except subprocess.CalledProcessError as e:
            hookenv.log("Error ({}) running  {}. Output: {}".format(
                e.returncode, e.cmd, e.output))
            if die_on_error:
                sys.exit(e.returncode)


def execd_preinstall(execd_dir=None):
    """Run charm-pre-install for each module within execd_dir."""
    execd_run('charm-pre-install', execd_dir=execd_dir)
