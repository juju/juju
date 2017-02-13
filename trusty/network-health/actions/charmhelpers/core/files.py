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

__author__ = 'Jorge Niedbalski <niedbalski@ubuntu.com>'

import os
import subprocess


def sed(filename, before, after, flags='g'):
    """
    Search and replaces the given pattern on filename.

    :param filename: relative or absolute file path.
    :param before: expression to be replaced (see 'man sed')
    :param after: expression to replace with (see 'man sed')
    :param flags: sed-compatible regex flags in example, to make
    the  search and replace case insensitive, specify ``flags="i"``.
    The ``g`` flag is always specified regardless, so you do not
    need to remember to include it when overriding this parameter.
    :returns: If the sed command exit code was zero then return,
    otherwise raise CalledProcessError.
    """
    expression = r's/{0}/{1}/{2}'.format(before,
                                         after, flags)

    return subprocess.check_call(["sed", "-i", "-r", "-e",
                                  expression,
                                  os.path.expanduser(filename)])
