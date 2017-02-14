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

import sys

__author__ = "Jorge Niedbalski <jorge.niedbalski@canonical.com>"


def current_version():
    """Current system python version"""
    return sys.version_info


def current_version_string():
    """Current system python version as string major.minor.micro"""
    return "{0}.{1}.{2}".format(sys.version_info.major,
                                sys.version_info.minor,
                                sys.version_info.micro)
