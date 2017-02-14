#!/usr/bin/env python
# coding: utf-8

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
import subprocess
import sys

from charmhelpers.fetch import apt_install, apt_update
from charmhelpers.core.hookenv import charm_dir, log

__author__ = "Jorge Niedbalski <jorge.niedbalski@canonical.com>"


def pip_execute(*args, **kwargs):
    """Overriden pip_execute() to stop sys.path being changed.

    The act of importing main from the pip module seems to cause add wheels
    from the /usr/share/python-wheels which are installed by various tools.
    This function ensures that sys.path remains the same after the call is
    executed.
    """
    try:
        _path = sys.path
        try:
            from pip import main as _pip_execute
        except ImportError:
            apt_update()
            apt_install('python-pip')
            from pip import main as _pip_execute
        _pip_execute(*args, **kwargs)
    finally:
        sys.path = _path


def parse_options(given, available):
    """Given a set of options, check if available"""
    for key, value in sorted(given.items()):
        if not value:
            continue
        if key in available:
            yield "--{0}={1}".format(key, value)


def pip_install_requirements(requirements, constraints=None, **options):
    """Install a requirements file.

    :param constraints: Path to pip constraints file.
    http://pip.readthedocs.org/en/stable/user_guide/#constraints-files
    """
    command = ["install"]

    available_options = ('proxy', 'src', 'log', )
    for option in parse_options(options, available_options):
        command.append(option)

    command.append("-r {0}".format(requirements))
    if constraints:
        command.append("-c {0}".format(constraints))
        log("Installing from file: {} with constraints {} "
            "and options: {}".format(requirements, constraints, command))
    else:
        log("Installing from file: {} with options: {}".format(requirements,
                                                               command))
    pip_execute(command)


def pip_install(package, fatal=False, upgrade=False, venv=None,
                constraints=None, **options):
    """Install a python package"""
    if venv:
        venv_python = os.path.join(venv, 'bin/pip')
        command = [venv_python, "install"]
    else:
        command = ["install"]

    available_options = ('proxy', 'src', 'log', 'index-url', )
    for option in parse_options(options, available_options):
        command.append(option)

    if upgrade:
        command.append('--upgrade')

    if constraints:
        command.extend(['-c', constraints])

    if isinstance(package, list):
        command.extend(package)
    else:
        command.append(package)

    log("Installing {} package with options: {}".format(package,
                                                        command))
    if venv:
        subprocess.check_call(command)
    else:
        pip_execute(command)


def pip_uninstall(package, **options):
    """Uninstall a python package"""
    command = ["uninstall", "-q", "-y"]

    available_options = ('proxy', 'log', )
    for option in parse_options(options, available_options):
        command.append(option)

    if isinstance(package, list):
        command.extend(package)
    else:
        command.append(package)

    log("Uninstalling {} package with options: {}".format(package,
                                                          command))
    pip_execute(command)


def pip_list():
    """Returns the list of current python installed packages
    """
    return pip_execute(["list"])


def pip_create_virtualenv(path=None):
    """Create an isolated Python environment."""
    apt_install('python-virtualenv')

    if path:
        venv_path = path
    else:
        venv_path = os.path.join(charm_dir(), 'venv')

    if not os.path.exists(venv_path):
        subprocess.check_call(['virtualenv', venv_path])
