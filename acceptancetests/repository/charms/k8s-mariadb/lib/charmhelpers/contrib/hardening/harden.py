# Copyright 2016 Canonical Limited.
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

from collections import OrderedDict

from charmhelpers.core.hookenv import (
    config,
    log,
    DEBUG,
    WARNING,
)
from charmhelpers.contrib.hardening.host.checks import run_os_checks
from charmhelpers.contrib.hardening.ssh.checks import run_ssh_checks
from charmhelpers.contrib.hardening.mysql.checks import run_mysql_checks
from charmhelpers.contrib.hardening.apache.checks import run_apache_checks

_DISABLE_HARDENING_FOR_UNIT_TEST = False


def harden(overrides=None):
    """Hardening decorator.

    This is the main entry point for running the hardening stack. In order to
    run modules of the stack you must add this decorator to charm hook(s) and
    ensure that your charm config.yaml contains the 'harden' option set to
    one or more of the supported modules. Setting these will cause the
    corresponding hardening code to be run when the hook fires.

    This decorator can and should be applied to more than one hook or function
    such that hardening modules are called multiple times. This is because
    subsequent calls will perform auditing checks that will report any changes
    to resources hardened by the first run (and possibly perform compliance
    actions as a result of any detected infractions).

    :param overrides: Optional list of stack modules used to override those
                      provided with 'harden' config.
    :returns: Returns value returned by decorated function once executed.
    """
    if overrides is None:
        overrides = []

    def _harden_inner1(f):
        # As this has to be py2.7 compat, we can't use nonlocal.  Use a trick
        # to capture the dictionary that can then be updated.
        _logged = {'done': False}

        def _harden_inner2(*args, **kwargs):
            # knock out hardening via a config var; normally it won't get
            # disabled.
            if _DISABLE_HARDENING_FOR_UNIT_TEST:
                return f(*args, **kwargs)
            if not _logged['done']:
                log("Hardening function '%s'" % (f.__name__), level=DEBUG)
                _logged['done'] = True
            RUN_CATALOG = OrderedDict([('os', run_os_checks),
                                       ('ssh', run_ssh_checks),
                                       ('mysql', run_mysql_checks),
                                       ('apache', run_apache_checks)])

            enabled = overrides[:] or (config("harden") or "").split()
            if enabled:
                modules_to_run = []
                # modules will always be performed in the following order
                for module, func in six.iteritems(RUN_CATALOG):
                    if module in enabled:
                        enabled.remove(module)
                        modules_to_run.append(func)

                if enabled:
                    log("Unknown hardening modules '%s' - ignoring" %
                        (', '.join(enabled)), level=WARNING)

                for hardener in modules_to_run:
                    log("Executing hardening module '%s'" %
                        (hardener.__name__), level=DEBUG)
                    hardener()
            else:
                log("No hardening applied to '%s'" % (f.__name__), level=DEBUG)

            return f(*args, **kwargs)
        return _harden_inner2

    return _harden_inner1
