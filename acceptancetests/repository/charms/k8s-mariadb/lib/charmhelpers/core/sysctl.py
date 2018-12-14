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

import yaml

from subprocess import check_call

from charmhelpers.core.hookenv import (
    log,
    DEBUG,
    ERROR,
)

__author__ = 'Jorge Niedbalski R. <jorge.niedbalski@canonical.com>'


def create(sysctl_dict, sysctl_file):
    """Creates a sysctl.conf file from a YAML associative array

    :param sysctl_dict: a dict or YAML-formatted string of sysctl
                        options eg "{ 'kernel.max_pid': 1337 }"
    :type sysctl_dict: str
    :param sysctl_file: path to the sysctl file to be saved
    :type sysctl_file: str or unicode
    :returns: None
    """
    if type(sysctl_dict) is not dict:
        try:
            sysctl_dict_parsed = yaml.safe_load(sysctl_dict)
        except yaml.YAMLError:
            log("Error parsing YAML sysctl_dict: {}".format(sysctl_dict),
                level=ERROR)
            return
    else:
        sysctl_dict_parsed = sysctl_dict

    with open(sysctl_file, "w") as fd:
        for key, value in sysctl_dict_parsed.items():
            fd.write("{}={}\n".format(key, value))

    log("Updating sysctl_file: %s values: %s" % (sysctl_file, sysctl_dict_parsed),
        level=DEBUG)

    check_call(["sysctl", "-p", sysctl_file])
