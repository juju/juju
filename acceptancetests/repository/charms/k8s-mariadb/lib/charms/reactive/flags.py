from charmhelpers.cli import cmdline
from charmhelpers.core import hookenv
from charmhelpers.core import unitdata

from charms.reactive.bus import FlagWatch
from charms.reactive.trace import tracer


__all__ = [
    'set_flag',
    'clear_flag',
    'toggle_flag',
    'register_trigger',
    'is_flag_set',
    'all_flags_set',
    'any_flags_set',
    'get_flags',
    'set_state',  # DEPRECATED
    'remove_state',  # DEPRECATED
    'toggle_state',  # DEPRECATED
    'is_state',  # DEPRECATED
    'all_states',  # DEPRECATED
    'get_states',  # DEPRECATED
    'any_states',  # DEPRECATED
]


class State(str):
    """
    .. deprecated:: 0.5.0
       Use flag instead

    A reactive state that can be set.

    States are essentially just strings, but this class should be used to enable them
    to be discovered and introspected, for documentation, composition, or linting.

    This should be used with :class:`StateList`.
    """
    pass


class StateList(object):
    """
    .. deprecated:: 0.5.0
       use flag instead

    Base class for a set of states that can be set by a relation or layer.

    This class should be used so that they can be discovered and introspected,
    for documentation, composition, or linting.

    Example usage::

        class MyRelation(RelationBase):
            class states(StateList):
                connected = State('{relation_name}.connected')
                available = State('{relation_name}.available')
    """
    pass


@cmdline.subcommand()
@cmdline.no_output
def set_flag(flag, value=None):
    """set_flag(flag)
    Set the given flag as active.

    :param str flag: Name of flag to set.

    .. note:: **Changes to flags are reset when a handler crashes.** Changes to
       flags happen immediately, but they are only persisted at the end of a
       complete and successful run of the reactive framework. All unpersisted
       changes are discarded when a hook crashes.
    """
    old_flags = get_flags()
    unitdata.kv().update({flag: value}, prefix='reactive.states.')
    if flag not in old_flags:
        tracer().set_flag(flag)
        FlagWatch.change(flag)
        trigger = _get_trigger(flag)
        for flag_name in trigger['set_flag']:
            set_flag(flag_name)
        for flag_name in trigger['clear_flag']:
            clear_flag(flag_name)


@cmdline.subcommand()
@cmdline.no_output
def clear_flag(flag):
    """
    Clear / deactivate a flag.

    :param str flag: Name of flag to set.

    .. note:: **Changes to flags are reset when a handler crashes.** Changes to
       flags happen immediately, but they are only persisted at the end of a
       complete and successful run of the reactive framework. All unpersisted
       changes are discarded when a hook crashes.
    """
    old_flags = get_flags()
    unitdata.kv().unset('reactive.states.%s' % flag)
    unitdata.kv().set('reactive.dispatch.removed_state', True)
    if flag in old_flags:
        tracer().clear_flag(flag)
        FlagWatch.change(flag)


@cmdline.subcommand()
@cmdline.no_output
def toggle_flag(flag, should_set):
    """
    Helper that calls either :func:`set_flag` or :func:`clear_flag`,
    depending on the value of `should_set`.

    Equivalent to::

        if should_set:
            set_flag(flag)
        else:
            clear_flag(flag)

    :param str flag: Name of flag to toggle.
    :param bool should_set: Whether to set the flag, or clear it.

    .. note:: **Changes to flags are reset when a handler crashes.** Changes to
       flags happen immediately, but they are only persisted at the end of a
       complete and successful run of the reactive framework. All unpersisted
       changes are discarded when a hook crashes.
    """
    if should_set:
        set_flag(flag)
    else:
        clear_flag(flag)


@cmdline.subcommand()
@cmdline.no_output
def register_trigger(when, set_flag=None, clear_flag=None):
    """
    Register a trigger to set or clear a flag when a given flag is set.

    Note: Flag triggers are handled at the same time that the given flag is set.

    :param str when: Flag to trigger on.
    :param str set_flag: If given, this flag will be set when `when` is set.
    :param str clear_flag: If given, this flag will be cleared when `when` is set.
    """
    trigger = _get_trigger(when)
    if set_flag and set_flag not in trigger['set_flag']:
        trigger['set_flag'].append(set_flag)
    if clear_flag and clear_flag not in trigger['clear_flag']:
        trigger['clear_flag'].append(clear_flag)
    _save_trigger(when, trigger)


def _get_trigger(when):
    key = 'reactive.flag_triggers.{}'.format(when)
    return unitdata.kv().get(key, {
        'set_flag': [],
        'clear_flag': [],
    })


def _save_trigger(when, data):
    key = 'reactive.flag_triggers.{}'.format(when)
    return unitdata.kv().set(key, data)


@cmdline.subcommand()
@cmdline.test_command
def is_flag_set(flag):
    """Assert that a flag is set"""
    return any_flags_set(flag)


@cmdline.subcommand()
@cmdline.test_command
def all_flags_set(*desired_flags):
    """Assert that all desired_flags are set"""
    active_flags = get_flags()
    return all(flag in active_flags for flag in desired_flags)


@cmdline.subcommand()
@cmdline.test_command
def any_flags_set(*desired_flags):
    """Assert that any of the desired_flags are set"""
    active_flags = get_flags()
    return any(flag in active_flags for flag in desired_flags)


@cmdline.subcommand()
def get_flags():
    """
    Return a list of all flags which are set.
    """
    flags = unitdata.kv().getrange('reactive.states.', strip=True) or {}
    return sorted(flags.keys())


def _get_flag_value(flag, default=None):
    return unitdata.kv().get('reactive.states.%s' % flag, default)


# DEPRECATED

@cmdline.subcommand()
@cmdline.no_output
def set_state(state, value=None):
    """
    .. deprecated:: 0.5.0
       Alias of :func:`set_flag`.
    """
    set_flag(state, value)


@cmdline.subcommand()
@cmdline.no_output
def remove_state(state):
    """
    .. deprecated:: 0.5.0
       Alias of :func:`clear_flag`"""
    clear_flag(state)


def toggle_state(state, should_set):
    """
    .. deprecated:: 0.5.0
       Alias of :func:`toggle_flag`"""
    toggle_flag(state, should_set)


@cmdline.subcommand()
@cmdline.test_command
def is_state(state):
    """
    .. deprecated:: 0.5.0
       Alias for :func:`is_flag_set`"""
    return is_flag_set(state)


@cmdline.subcommand()
@cmdline.test_command
def all_states(*desired_states):
    """
    .. deprecated:: 0.5.0
       Alias for :func:`all_flags_set`"""
    return all_flags_set(*desired_states)


@cmdline.subcommand()
@cmdline.test_command
def any_states(*desired_states):
    """
    .. deprecated:: 0.5.0
       Alias for :func:`any_flags_set`"""
    return any_flags_set(*desired_states)


@cmdline.subcommand()
def get_states():
    """
    .. deprecated:: 0.5.0
       Use :func:`get_flags` instead.

    Return a mapping of all active states to their values.
    """
    return unitdata.kv().getrange('reactive.states.', strip=True) or {}


def get_state(flag, default=None):
    """
    .. deprecated:: 0.5.0
       For internal use only.
    """
    return _get_flag_value(flag, default)


# INTERNAL

@hookenv.atstart
def _manage_automatic_flags():
    _manage_upgrade_flags()


def _manage_upgrade_flags():
    hook_name = hookenv.hook_name()

    if hook_name == 'pre-series-upgrade':
        set_flag('upgrade.series.in-progress')

    if hook_name == 'post-series-upgrade':
        clear_flag('upgrade.series.in-progress')
