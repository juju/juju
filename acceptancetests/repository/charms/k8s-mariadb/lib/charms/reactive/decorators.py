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

from functools import wraps, partial
from inspect import getfile, isclass, signature
from pathlib import Path

from charmhelpers.core import hookenv
from charms.reactive.bus import Handler
from charms.reactive.bus import _action_id
from charms.reactive.bus import _short_action_id
from charms.reactive.flags import get_flags
from charms.reactive.relations import endpoint_from_name
from charms.reactive.relations import endpoint_from_flag
from charms.reactive.endpoints import Endpoint
from charms.reactive.helpers import _hook
from charms.reactive.helpers import _restricted_hook
from charms.reactive.helpers import _when_all
from charms.reactive.helpers import _when_any
from charms.reactive.helpers import _when_none
from charms.reactive.helpers import _when_not_all
from charms.reactive.helpers import any_file_changed
from charms.reactive.helpers import was_invoked
from charms.reactive.helpers import mark_invoked


__all__ = [
    'when',
    'when_all',
    'when_any',
    'when_not',
    'when_none',
    'when_not_all',
    'not_unless',
    'when_file_changed',
    'collect_metrics',
    'meter_status_changed',
    'only_once',  # DEPRECATED
    'hook',  # DEPRECATED
]


def hook(*hook_patterns):
    """
    Register the decorated function to run when the current hook matches any of
    the ``hook_patterns``.

    This decorator is generally deprecated and should only be used when
    absolutely necessary.

    The hook patterns can use the ``{interface:...}`` and ``{A,B,...}`` syntax
    supported by :func:`~charms.reactive.bus.any_hook`.

    Note that hook decorators **cannot** be combined with :func:`when` or
    :func:`when_not` decorators.
    """
    def _register(action):
        def arg_gen():
            # use a generator to defer calling of hookenv.relation_type, for tests
            rel = endpoint_from_name(hookenv.relation_type())
            if rel:
                yield rel

        handler = Handler.get(action)
        handler.add_predicate(partial(_hook, hook_patterns))
        handler.add_args(arg_gen())
        return action
    return _register


def _when_decorator(predicate, desired_flags, action, legacy_args=False):
    endpoint_names = _get_endpoint_names(action)
    has_relname_flag = _has_endpoint_name_flag(desired_flags)
    params = signature(action).parameters
    has_params = len(params) > 0
    if has_relname_flag and not endpoint_names:
        # If this is an Endpoint handler but there are no endpoints
        # for this interface & role, then we shouldn't register its
        # handlers.  It probably means we only use a different role.
        return action
    for endpoint_name in endpoint_names or [None]:
        handler = Handler.get(action, endpoint_name)
        flags = _expand_endpoint_name(endpoint_name, desired_flags)
        handler.add_predicate(partial(predicate, flags))
        if _is_endpoint_method(action):
            # Endpoint handler methods expect self to be passed in to conform
            # to instance method convention. But mutliple decorators should
            # take care to not pass in multiple copies of self.
            if not handler.has_args:
                handler.add_args(map(endpoint_from_name, [endpoint_name]))
        elif has_params and legacy_args:
            # Handlers should all move to not taking any params and getting
            # the Endpoint instances from the context, but during the
            # transition, we need to provide for handlers expecting args.
            handler.add_args(filter(None, map(endpoint_from_flag, flags)))
        handler.register_flags(flags)
    return action


def when(*desired_flags):
    """
    Alias for :func:`~charms.reactive.decorators.when_all`.
    """
    return when_all(*desired_flags)


def when_all(*desired_flags):
    """
    Register the decorated function to run when all of ``desired_flags`` are active.

    Note that handlers whose conditions match are triggered at least once per
    hook invocation.

    For backwards compatibility, this decorator can pass arguments, but it is
    recommended to use argument-less handlers.  See
    `the summary <#charms-reactive-decorators>`_ for more information.
    """
    return partial(_when_decorator, _when_all, desired_flags, legacy_args=True)


def when_any(*desired_flags):
    """
    Register the decorated function to run when any of ``desired_flags`` are active.

    This decorator will never cause arguments to be passed to the handler.

    Note that handlers whose conditions match are triggered at least once per
    hook invocation.
    """
    return partial(_when_decorator, _when_any, desired_flags, legacy_args=False)


def when_not(*desired_flags):
    """
    Alias for :func:`~charms.reactive.decorators.when_none`.
    """
    return when_none(*desired_flags)


def when_none(*desired_flags):
    """
    Register the decorated function to run when none of ``desired_flags`` are
    active.

    This decorator will never cause arguments to be passed to the handler.

    Note that handlers whose conditions match are triggered at least once per
    hook invocation.
    """
    return partial(_when_decorator, _when_none, desired_flags, legacy_args=False)


def when_not_all(*desired_flags):
    """
    Register the decorated function to run when one or more of the
    ``desired_flags`` are not active.

    This decorator will never cause arguments to be passed to the handler.

    Note that handlers whose conditions match are triggered at least once per
    hook invocation.
    """
    return partial(_when_decorator, _when_not_all, desired_flags, legacy_args=False)


def when_file_changed(*filenames, **kwargs):
    """
    Register the decorated function to run when one or more files have changed.

    :param list filenames: The names of one or more files to check for changes
        (a callable returning the name is also accepted).
    :param str hash_type: The type of hash to use for determining if a file has
        changed.  Defaults to 'md5'.  Must be given as a kwarg.
    """
    def _register(action):
        handler = Handler.get(action)
        handler.add_predicate(partial(any_file_changed, filenames, **kwargs))
        return action
    return _register


def not_unless(*desired_flags):
    """
    Assert that the decorated function can only be called if the desired_flags
    are active.

    Note that, unlike :func:`when`, this does **not** trigger the decorated
    function if the flags match.  It **only** raises an exception if the
    function is called when the flags do not match.

    This is primarily for informational purposes and as a guard clause.
    """
    def _decorator(func):
        action_id = _action_id(func)
        short_action_id = _short_action_id(func)

        @wraps(func)
        def _wrapped(*args, **kwargs):
            active_flags = get_flags()
            missing_flags = [flag for flag in desired_flags if flag not in active_flags]
            if missing_flags:
                hookenv.log('%s called before flag%s: %s' % (
                    short_action_id,
                    's' if len(missing_flags) > 1 else '',
                    ', '.join(missing_flags)), hookenv.WARNING)
            return func(*args, **kwargs)
        _wrapped._action_id = action_id
        _wrapped._short_action_id = short_action_id
        return _wrapped
    return _decorator


def only_once(action=None):
    """
    .. deprecated:: 0.5.0
       Use :func:`when_not` in combination with :func:`set_state` instead. This
       handler is deprecated because it might actually be
       `called multiple times <https://github.com/juju-solutions/charms.reactive/issues/22>`_.

    Register the decorated function to be run once, and only once.

    This decorator will never cause arguments to be passed to the handler.
    """
    if action is None:
        # allow to be used as @only_once or @only_once()
        return only_once

    action_id = _action_id(action)
    handler = Handler.get(action)
    handler.add_predicate(lambda: not was_invoked(action_id))
    handler.add_post_callback(partial(mark_invoked, action_id))
    return action


def collect_metrics():
    """
    Register the decorated function to run for the collect_metrics hook.
    """
    def _register(action):
        handler = Handler.get(action)
        handler.add_predicate(partial(_restricted_hook, 'collect-metrics'))
        return action
    return _register


def meter_status_changed():
    """
    Register the decorated function to run when a meter status change has been detected.
    """
    def _register(action):
        handler = Handler.get(action)
        handler.add_predicate(partial(_restricted_hook, 'meter-status-changed'))
        return action
    return _register


def _has_endpoint_name_flag(flags):
    """
    Detect if the given flags contain any that use ``{endpoint_name}``.
    """
    return '{endpoint_name}' in ''.join(flags)


def _get_endpoint_names(handler):
    filepath = Path(getfile(handler))
    role = filepath.stem
    interface = filepath.parent.name
    if role not in ('requires', 'provides', 'peers'):
        return []
    endpoint_names = hookenv.role_and_interface_to_relations(role, interface)
    if not endpoint_names:
        return []
    return endpoint_names


def _expand_endpoint_name(endpoint_name, flags):
    """
    Populate any ``{endpoint_name}`` tags in the flag names for the given
    handler, based on the handlers module / file name.
    """
    return tuple(flag.format(endpoint_name=endpoint_name) for flag in flags)


def _is_endpoint_method(handler):
    """
    from the context.  Unfortunately, we can't directly detect whether
    a handler is an Endpoint method, because at the time of decoration,
    the class doesn't actually exist yet so it's impossible to get a
    reference to it.  So, we use the heuristic of seeing if the handler
    takes only a single ``self`` param and there is an Endpoint class in
    the handler's globals.
    """
    params = signature(handler).parameters
    has_self = len(params) == 1 and list(params.keys())[0] == 'self'
    has_endpoint_class = any(isclass(g) and issubclass(g, Endpoint)
                             for g in handler.__globals__.values())
    return has_self and has_endpoint_class
