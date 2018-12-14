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
        msg = "Unable to interpret non-string value '%s' as bytes" % (value)
        raise ValueError(msg)
    matches = re.match("([0-9]+)([a-zA-Z]+)", value)
    if matches:
        size = int(matches.group(1)) * (1024 ** BYTE_POWER[matches.group(2)])
    else:
        # Assume that value passed in is bytes
        try:
            size = int(value)
        except ValueError:
            msg = "Unable to interpret string value '%s' as bytes" % (value)
            raise ValueError(msg)
    return size


class BasicStringComparator(object):
    """Provides a class that will compare strings from an iterator type object.
    Used to provide > and < comparisons on strings that may not necessarily be
    alphanumerically ordered.  e.g. OpenStack or Ubuntu releases AFTER the
    z-wrap.
    """

    _list = None

    def __init__(self, item):
        if self._list is None:
            raise Exception("Must define the _list in the class definition!")
        try:
            self.index = self._list.index(item)
        except Exception:
            raise KeyError("Item '{}' is not in list '{}'"
                           .format(item, self._list))

    def __eq__(self, other):
        assert isinstance(other, str) or isinstance(other, self.__class__)
        return self.index == self._list.index(other)

    def __ne__(self, other):
        return not self.__eq__(other)

    def __lt__(self, other):
        assert isinstance(other, str) or isinstance(other, self.__class__)
        return self.index < self._list.index(other)

    def __ge__(self, other):
        return not self.__lt__(other)

    def __gt__(self, other):
        assert isinstance(other, str) or isinstance(other, self.__class__)
        return self.index > self._list.index(other)

    def __le__(self, other):
        return not self.__gt__(other)

    def __str__(self):
        """Always give back the item at the index so it can be used in
        comparisons like:

        s_mitaka = CompareOpenStack('mitaka')
        s_newton = CompareOpenstack('newton')

        assert s_newton > s_mitaka

        @returns: <string>
        """
        return self._list[self.index]
