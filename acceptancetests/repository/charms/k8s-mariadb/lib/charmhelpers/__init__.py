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

# Bootstrap charm-helpers, installing its dependencies if necessary using
# only standard libraries.
from __future__ import print_function
from __future__ import absolute_import

import functools
import inspect
import subprocess
import sys

try:
    import six  # NOQA:F401
except ImportError:
    if sys.version_info.major == 2:
        subprocess.check_call(['apt-get', 'install', '-y', 'python-six'])
    else:
        subprocess.check_call(['apt-get', 'install', '-y', 'python3-six'])
    import six  # NOQA:F401

try:
    import yaml  # NOQA:F401
except ImportError:
    if sys.version_info.major == 2:
        subprocess.check_call(['apt-get', 'install', '-y', 'python-yaml'])
    else:
        subprocess.check_call(['apt-get', 'install', '-y', 'python3-yaml'])
    import yaml  # NOQA:F401


# Holds a list of mapping of mangled function names that have been deprecated
# using the @deprecate decorator below.  This is so that the warning is only
# printed once for each usage of the function.
__deprecated_functions = {}


def deprecate(warning, date=None, log=None):
    """Add a deprecation warning the first time the function is used.
    The date, which is a string in semi-ISO8660 format indicate the year-month
    that the function is officially going to be removed.

    usage:

    @deprecate('use core/fetch/add_source() instead', '2017-04')
    def contributed_add_source_thing(...):
        ...

    And it then prints to the log ONCE that the function is deprecated.
    The reason for passing the logging function (log) is so that hookenv.log
    can be used for a charm if needed.

    :param warning:  String to indicat where it has moved ot.
    :param date: optional sting, in YYYY-MM format to indicate when the
                 function will definitely (probably) be removed.
    :param log: The log function to call to log.  If not, logs to stdout
    """
    def wrap(f):

        @functools.wraps(f)
        def wrapped_f(*args, **kwargs):
            try:
                module = inspect.getmodule(f)
                file = inspect.getsourcefile(f)
                lines = inspect.getsourcelines(f)
                f_name = "{}-{}-{}..{}-{}".format(
                    module.__name__, file, lines[0], lines[-1], f.__name__)
            except (IOError, TypeError):
                # assume it was local, so just use the name of the function
                f_name = f.__name__
            if f_name not in __deprecated_functions:
                __deprecated_functions[f_name] = True
                s = "DEPRECATION WARNING: Function {} is being removed".format(
                    f.__name__)
                if date:
                    s = "{} on/around {}".format(s, date)
                if warning:
                    s = "{} : {}".format(s, warning)
                if log:
                    log(s)
                else:
                    print(s)
            return f(*args, **kwargs)
        return wrapped_f
    return wrap
