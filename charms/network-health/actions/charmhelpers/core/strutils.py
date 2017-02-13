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

import six
import re


def bool_from_string(value):
    """Interpret string value as boolean.

    Returns True if value translates to True otherwise False.
    """
    if isinstance(value, six.string_types):
        value = six.text_type(value)
    else:
        msg = "Unable to interpret non-string value '%s' as boolean" % (value)
        raise ValueError(msg)

    value = value.strip().lower()

    if value in ['y', 'yes', 'true', 't', 'on']:
        return True
    elif value in ['n', 'no', 'false', 'f', 'off']:
        return False

    msg = "Unable to interpret string value '%s' as boolean" % (value)
    raise ValueError(msg)


def bytes_from_string(value):
    """Interpret human readable string value as bytes.

    Returns int
    """
    BYTE_POWER = {
        'K': 1,
        'KB': 1,
        'M': 2,
        'MB': 2,
        'G': 3,
        'GB': 3,
        'T': 4,
        'TB': 4,
        'P': 5,
        'PB': 5,
    }
    if isinstance(value, six.string_types):
        value = six.text_type(value)
    else:
        msg = "Unable to interpret non-string value '%s' as boolean" % (value)
        raise ValueError(msg)
    matches = re.match("([0-9]+)([a-zA-Z]+)", value)
    if not matches:
        msg = "Unable to interpret string value '%s' as bytes" % (value)
        raise ValueError(msg)
    return int(matches.group(1)) * (1024 ** BYTE_POWER[matches.group(2)])
