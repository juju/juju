# Copyright 2014-2017 Canonical Limited.
#
# This file is part of charms.reactive
#
# charms.reactive is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charms.reactive is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import os
import traceback

from .flags import *  # noqa
from .relations import *  # noqa
from .endpoints import *  # noqa
from .decorators import *  # noqa
from .helpers import *  # noqa

from . import bus
from . import flags
from . import helpers
from charmhelpers.core import hookenv
from charmhelpers.core import unitdata


# DEPRECATED: transitional imports for backwards compatibility
bus.StateList = flags.StateList
bus.State = flags.State
bus.set_state = flags.set_flag
bus.remove_state = flags.clear_flag
bus.get_state = flags.get_state
bus.get_states = flags.get_states
helpers.is_state = flags.is_flag_set
helpers.toggle_state = flags.toggle_flag
helpers.all_states = flags.all_flags_set
helpers.any_states = flags.any_flags_set


def main(relation_name=None):
    """
    This is the main entry point for the reactive framework.  It calls
    :func:`~bus.discover` to find and load all reactive handlers (e.g.,
    :func:`@when <decorators.when>` decorated blocks), and then
    :func:`~bus.dispatch` to trigger handlers until the queue settles out.
    Finally, :meth:`unitdata.kv().flush <charmhelpers.core.unitdata.Storage.flush>`
    is called to persist the flags and other data.

    :param str relation_name: Optional name of the relation which is being handled.
    """
    hook_name = hookenv.hook_name()
    restricted_mode = hook_name in ['meter-status-changed', 'collect-metrics']

    hookenv.log('Reactive main running for hook %s' % hookenv.hook_name(), level=hookenv.INFO)
    if restricted_mode:
        hookenv.log('Restricted mode.', level=hookenv.INFO)

    # work-around for https://bugs.launchpad.net/juju-core/+bug/1503039
    # ensure that external handlers can tell what hook they're running in
    if 'JUJU_HOOK_NAME' not in os.environ:
        os.environ['JUJU_HOOK_NAME'] = hook_name

    try:
        bus.discover()
        if not restricted_mode:  # limit what gets run in restricted mode
            hookenv._run_atstart()
        bus.dispatch(restricted=restricted_mode)
    except Exception:
        tb = traceback.format_exc()
        hookenv.log('Hook error:\n{}'.format(tb), level=hookenv.ERROR)
        raise
    except SystemExit as x:
        if x.code not in (None, 0):
            raise

    if not restricted_mode:  # limit what gets run in restricted mode
        hookenv._run_atexit()
    unitdata._KV.flush()
