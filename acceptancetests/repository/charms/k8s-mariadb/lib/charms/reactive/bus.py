# Copyright 2014-2017 Canonical Limited.
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

import importlib
import os
import sys
import errno
import subprocess
from itertools import chain
from functools import partial

from charmhelpers.core import hookenv
from charmhelpers.core import unitdata
from charms.reactive.trace import tracer


_log_opts = os.environ.get('REACTIVE_LOG_OPTS', '').split(',')
LOG_OPTS = {
    'register': 'register' in _log_opts,
}


class BrokenHandlerException(Exception):
    def __init__(self, path):
        message = ("File at '{}' is marked as executable but "
                   "execution failed. Only handler files may be marked "
                   "as executable.".format(path))
        super(BrokenHandlerException, self).__init__(message)


def _action_id(action, suffix=None):
    if hasattr(action, '_action_id'):
        return action._action_id
    parts = [
        action.__code__.co_filename,
        action.__code__.co_firstlineno,
        action.__code__.co_name,
    ]
    if suffix is not None:
        parts.append(suffix)
    return ':'.join(map(str, parts))


def _short_action_id(action, suffix=None):
    if hasattr(action, '_short_action_id'):
        return action._short_action_id
    filepath = os.path.relpath(action.__code__.co_filename, hookenv.charm_dir())
    parts = [
        filepath,
        action.__code__.co_firstlineno,
        action.__code__.co_name,
    ]
    if suffix is not None:
        parts.append(suffix)
    return ':'.join(map(str, parts))


class Handler(object):
    """
    Class representing a reactive flag handler.
    """
    _HANDLERS = {}
    _CONSUMED_FLAGS = set()

    @classmethod
    def get(cls, action, suffix=None):
        """
        Get or register a handler for the given action.

        :param func action: Callback that is called when invoking the Handler
        :param func suffix: Optional suffix for the handler's ID
        """
        action_id = _action_id(action, suffix)
        if action_id not in cls._HANDLERS:
            if LOG_OPTS['register']:
                hookenv.log('Registering reactive handler for %s' % _short_action_id(action, suffix),
                            level=hookenv.DEBUG)
            cls._HANDLERS[action_id] = cls(action, suffix)
        return cls._HANDLERS[action_id]

    @classmethod
    def get_handlers(cls):
        """
        Get all registered handlers.
        """
        return cls._HANDLERS.values()

    @classmethod
    def clear(cls):
        """
        Clear all registered handlers.
        """
        cls._HANDLERS = {}

    def __init__(self, action, suffix=None):
        """
        Create a new Handler.

        :param func action: Callback that is called when invoking the Handler
        :param func suffix: Optional suffix for the handler's ID
        """
        self._action_id = _short_action_id(action, suffix)
        self._action = action
        self._args = []
        self._predicates = []
        self._post_callbacks = []
        self._flags = set()

    def id(self):
        return self._action_id

    def add_args(self, args):
        """
        Add arguments to be passed to the action when invoked.

        :param args: Any sequence or iterable, which will be lazily evaluated
            to provide args.  Subsequent calls to :meth:`add_args` can be used
            to add additional arguments.
        """
        self._args.append(args)

    @property
    def has_args(self):
        """
        Whether or not this Handler has had any args added via :meth:`add_args`.
        """
        return len(self._args) > 0

    def add_predicate(self, predicate):
        """
        Add a new predicate callback to this handler.
        """
        _predicate = predicate
        if isinstance(predicate, partial):
            _predicate = 'partial(%s, %s, %s)' % (predicate.func, predicate.args, predicate.keywords)
        if LOG_OPTS['register']:
            hookenv.log('  Adding predicate for %s: %s' % (self.id(), _predicate), level=hookenv.DEBUG)
        self._predicates.append(predicate)

    def add_post_callback(self, callback):
        """
        Add a callback to be run after the action is invoked.
        """
        self._post_callbacks.append(callback)

    def test(self):
        """
        Check the predicate(s) and return True if this handler should be invoked.
        """
        if self._flags and not FlagWatch.watch(self._action_id, self._flags):
            return False
        return all(predicate() for predicate in self._predicates)

    def _get_args(self):
        """
        Lazily evaluate the args.
        """
        if not hasattr(self, '_args_evaled'):
            # cache the args in case handler is re-invoked due to flags change
            self._args_evaled = list(chain.from_iterable(self._args))
        return self._args_evaled

    def invoke(self):
        """
        Invoke this handler.
        """
        args = self._get_args()
        self._action(*args)
        for callback in self._post_callbacks:
            callback()

    def register_flags(self, flags):
        """
        Register flags as being relevant to this handler.

        Relevant flags will be used to determine if the handler should
        be re-invoked due to changes in the set of active flags.  If this
        handler has already been invoked during this :func:`dispatch` run
        and none of its relevant flags have been set or removed since then,
        then the handler will be skipped.

        This is also used for linting and composition purposes, to determine
        if a layer has unhandled flags.
        """
        self._CONSUMED_FLAGS.update(flags)
        self._flags.update(flags)


class ExternalHandler(Handler):
    """
    A variant Handler for external executable actions (such as bash scripts).

    External handlers must adhere to the following protocol:

      * The handler can be any executable

      * When invoked with the ``--test`` command-line flag, it should exit with
        an exit code of zero to indicate that the handler should be invoked, and
        a non-zero exit code to indicate that it need not be invoked.  It can
        also provide a line of output to be passed to the ``--invoke`` call, e.g.,
        to indicate which sub-handlers should be invoked.  The handler should
        **not** perform its action when given this flag.

      * When invoked with the ``--invoke`` command-line flag (which will be
        followed by any output returned by the ``--test`` call), the handler
        should perform its action(s).
    """
    @classmethod
    def register(cls, filepath):
        if filepath not in Handler._HANDLERS:
            _filepath = os.path.relpath(filepath, hookenv.charm_dir())
            if LOG_OPTS['register']:
                hookenv.log('Registering external reactive handler for %s' % _filepath, level=hookenv.DEBUG)
            Handler._HANDLERS[filepath] = cls(filepath)
        return Handler._HANDLERS[filepath]

    def __init__(self, filepath):
        self._filepath = filepath
        self._test_output = ''

    def id(self):
        _filepath = os.path.relpath(self._filepath, hookenv.charm_dir())
        return '%s "%s"' % (_filepath, self._test_output)

    def test(self):
        """
        Call the external handler to test whether it should be invoked.
        """
        # flush to ensure external process can see flags as they currently
        # are, and write flags (flush releases lock)
        unitdata.kv().flush()
        try:
            proc = subprocess.Popen([self._filepath, '--test'], stdout=subprocess.PIPE, env=os.environ)
        except OSError as oserr:
            if oserr.errno == errno.ENOEXEC:
                raise BrokenHandlerException(self._filepath)
            raise
        self._test_output, _ = proc.communicate()
        return proc.returncode == 0

    def invoke(self):
        """
        Call the external handler to be invoked.
        """
        # flush to ensure external process can see flags as they currently
        # are, and write flags (flush releases lock)
        unitdata.kv().flush()
        subprocess.check_call([self._filepath, '--invoke', self._test_output], env=os.environ)


class FlagWatch(object):
    key = 'reactive.state_watch'

    @classmethod
    def _store(cls):
        return unitdata.kv()

    @classmethod
    def _get(cls):
        return cls._store().get(cls.key, {
            'iteration': 0,
            'changes': [],
            'pending': [],
        })

    @classmethod
    def _set(cls, data):
        cls._store().set(cls.key, data)

    @classmethod
    def reset(cls):
        cls._store().unset(cls.key)

    @classmethod
    def iteration(cls, i):
        data = cls._get()
        data['iteration'] = i
        cls._set(data)

    @classmethod
    def watch(cls, watcher, flags):
        data = cls._get()
        iteration = data['iteration']
        changed = bool(set(flags) & set(data['changes']))
        return iteration == 0 or changed

    @classmethod
    def change(cls, flag):
        data = cls._get()
        data['pending'].append(flag)
        cls._set(data)

    @classmethod
    def commit(cls):
        data = cls._get()
        data['changes'] = data['pending']
        data['pending'] = []
        cls._set(data)


def dispatch(restricted=False):
    """
    Dispatch registered handlers.

    When dispatching in restricted mode, only matching hook handlers are executed.

    Handlers are dispatched according to the following rules:

    * Handlers are repeatedly tested and invoked in iterations, until the system
      settles into quiescence (that is, until no new handlers match to be invoked).

    * In the first iteration, :func:`@hook <charms.reactive.decorators.hook>`
      and :func:`@action <charms.reactive.decorators.action>` handlers will
      be invoked, if they match.

    * In subsequent iterations, other handlers are invoked, if they match.

    * Added flags will not trigger new handlers until the next iteration,
      to ensure that chained flags are invoked in a predictable order.

    * Removed flags will cause the current set of matched handlers to be
      re-tested, to ensure that no handler is invoked after its matching
      flag has been removed.

    * Other than the guarantees mentioned above, the order in which matching
      handlers are invoked is undefined.

    * Flags are preserved between hook and action invocations, and all matching
      handlers are re-invoked for every hook and action.  There are
      :doc:`decorators <charms.reactive.decorators>` and
      :doc:`helpers <charms.reactive.helpers>`
      to prevent unnecessary reinvocations, such as
      :func:`~charms.reactive.decorators.only_once`.
    """
    FlagWatch.reset()

    def _test(to_test):
        return list(filter(lambda h: h.test(), to_test))

    def _invoke(to_invoke):
        while to_invoke:
            unitdata.kv().set('reactive.dispatch.removed_state', False)
            for handler in list(to_invoke):
                to_invoke.remove(handler)
                hookenv.log('Invoking reactive handler: %s' % handler.id(), level=hookenv.INFO)
                handler.invoke()
                if unitdata.kv().get('reactive.dispatch.removed_state'):
                    # re-test remaining handlers
                    to_invoke = _test(to_invoke)
                    break
        FlagWatch.commit()

    tracer().start_dispatch()

    # When in restricted context, only run hooks for that context.
    if restricted:
        unitdata.kv().set('reactive.dispatch.phase', 'restricted')
        hook_handlers = _test(Handler.get_handlers())
        tracer().start_dispatch_phase('restricted', hook_handlers)
        _invoke(hook_handlers)
        return

    unitdata.kv().set('reactive.dispatch.phase', 'hooks')
    hook_handlers = _test(Handler.get_handlers())
    tracer().start_dispatch_phase('hooks', hook_handlers)
    _invoke(hook_handlers)

    unitdata.kv().set('reactive.dispatch.phase', 'other')
    for i in range(100):
        FlagWatch.iteration(i)
        other_handlers = _test(Handler.get_handlers())
        if i == 0:
            tracer().start_dispatch_phase('other', other_handlers)
        tracer().start_dispatch_iteration(i, other_handlers)
        if not other_handlers:
            break
        _invoke(other_handlers)

    FlagWatch.reset()


def discover():
    """
    Discover handlers based on convention.

    Handlers will be loaded from the following directories and their subdirectories:

      * ``$CHARM_DIR/reactive/``
      * ``$CHARM_DIR/hooks/reactive/``
      * ``$CHARM_DIR/hooks/relations/``

    They can be Python files, in which case they will be imported and decorated
    functions registered.  Or they can be executables, in which case they must
    adhere to the :class:`ExternalHandler` protocol.
    """
    # Add $CHARM_DIR and $CHARM_DIR/hooks to sys.path so
    # 'import reactive.leadership', 'import relations.pgsql' works
    # as expected, as well as relative imports like 'import ..leadership'
    # or 'from . import leadership'. Without this, it becomes difficult
    # for layers to access APIs provided by other layers. This addition
    # needs to remain in effect, in case discovered modules are doing
    # late imports.
    _append_path(hookenv.charm_dir())
    _append_path(os.path.join(hookenv.charm_dir(), 'hooks'))

    for search_dir in ('reactive', 'hooks/reactive', 'hooks/relations'):
        search_path = os.path.join(hookenv.charm_dir(), search_dir)
        for dirpath, dirnames, filenames in os.walk(search_path):
            for filename in filenames:
                filepath = os.path.join(dirpath, filename)
                _register_handlers_from_file(search_path, filepath)


def _append_path(d):
    if d not in sys.path:
        sys.path.append(d)


def _load_module(root, filepath):
    assert filepath.startswith(root + os.sep)
    assert filepath.endswith('.py')
    package = os.path.basename(root)  # 'reactive' or 'relations'
    assert package in ('reactive', 'relations')
    module = filepath[len(root):-3].replace(os.sep, '.')
    if module.endswith('.__init__'):
        module = module[:-9]

    # Standard import.
    return importlib.import_module(package + module)


def _register_handlers_from_file(root, filepath):
    no_exec_blacklist = (
        '.md', '.yaml', '.txt', '.ini',
        'makefile', '.gitignore',
        'copyright', 'license')
    if filepath.lower().endswith(no_exec_blacklist):
        # Don't load handlers with one of the blacklisted extensions
        return
    if filepath.endswith('.py'):
        _load_module(root, filepath)
    elif os.access(filepath, os.X_OK):
        ExternalHandler.register(filepath)
