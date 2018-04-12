#!/usr/bin/env python
# -*- coding: utf-8 -*-

# Copyright 2014-2015 Canonical Limited.
#
# This file is part of charm-helpers.
#
# charm-helpers is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charm-helpers is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import six


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
