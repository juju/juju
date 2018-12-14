#!/usr/bin/env python
# -*- coding: utf-8 -*-

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

import re
import subprocess

from charmhelpers.osplatform import get_platform
from charmhelpers.core.hookenv import (
    log,
    INFO
)

__platform__ = get_platform()
if __platform__ == "ubuntu":
    from charmhelpers.core.kernel_factory.ubuntu import (  # NOQA:F401
        persistent_modprobe,
        update_initramfs,
    )  # flake8: noqa -- ignore F401 for this import
elif __platform__ == "centos":
    from charmhelpers.core.kernel_factory.centos import (  # NOQA:F401
        persistent_modprobe,
        update_initramfs,
    )  # flake8: noqa -- ignore F401 for this import

__author__ = "Jorge Niedbalski <jorge.niedbalski@canonical.com>"


def modprobe(module, persist=True):
    """Load a kernel module and configure for auto-load on reboot."""
    cmd = ['modprobe', module]

    log('Loading kernel module %s' % module, level=INFO)

    subprocess.check_call(cmd)
    if persist:
        persistent_modprobe(module)


def rmmod(module, force=False):
    """Remove a module from the linux kernel"""
    cmd = ['rmmod']
    if force:
        cmd.append('-f')
    cmd.append(module)
    log('Removing kernel module %s' % module, level=INFO)
    return subprocess.check_call(cmd)


def lsmod():
    """Shows what kernel modules are currently loaded"""
    return subprocess.check_output(['lsmod'],
                                   universal_newlines=True)


def is_module_loaded(module):
    """Checks if a kernel module is already loaded"""
    matches = re.findall('^%s[ ]+' % module, lsmod(), re.M)
    return len(matches) > 0
