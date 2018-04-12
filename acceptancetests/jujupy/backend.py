# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2017 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

import logging
import json
import os
import pexpect
import subprocess

from contextlib import contextmanager
from datetime import datetime

from jujupy.exceptions import (
    CannotConnectEnv,
    NoActiveControllers,
    NoActiveModel,
    SoftDeadlineExceeded,
    )
from jujupy.utility import (
    get_timeout_path,
    get_timeout_prefix,
    pause,
    quote,
    scoped_environ,
    )
from jujupy.wait_condition import (
    CommandTime,
    )

__metaclass__ = type

# Python 2 and 3 compatibility
try:
    argtype = basestring
except NameError:
    argtype = str

log = logging.getLogger("jujupy.backend")

JUJU_DEV_FEATURE_FLAGS = 'JUJU_DEV_FEATURE_FLAGS'


class JujuBackend:
    """A Juju backend referring to a specific juju 2 binary.

    Uses -m to specify models, uses JUJU_DATA to specify home directory.
    """

    _model_flag = '-m'

    def __init__(self, full_path, version, feature_flags, debug,
                 soft_deadline=None):
        self._version = version
        self._full_path = full_path
        self.feature_flags = feature_flags
        self.debug = debug
        self._timeout_path = get_timeout_path()
        self.juju_timings = []
        self.soft_deadline = soft_deadline
        self._ignore_soft_deadline = False
        # List of ModelClients, keep track of models added so we can remove
        # only those added during a test run (i.e. when using an existing
        # controller.)
        self._added_models = []

    def _now(self):
        return datetime.utcnow()

    @contextmanager
    def _check_timeouts(self):
        # If an exception occurred, we don't want to replace it with
        # SoftDeadlineExceeded.
        yield
        if self.soft_deadline is None or self._ignore_soft_deadline:
            return
        if self._now() > self.soft_deadline:
            raise SoftDeadlineExceeded()

    @contextmanager
    def ignore_soft_deadline(self):
        """Ignore the client deadline.  For cleanup code."""
        old_val = self._ignore_soft_deadline
        self._ignore_soft_deadline = True
        try:
            yield
        finally:
            self._ignore_soft_deadline = old_val

    def clone(self, full_path, version, debug, feature_flags):
        if version is None:
            version = self.version
        if full_path is None:
            full_path = self.full_path
        if debug is None:
            debug = self.debug
        result = self.__class__(full_path, version, feature_flags, debug,
                                self.soft_deadline)
        # Each clone shares a reference to juju_timings allowing us to collect
        # all commands run during a test.
        result.juju_timings = self.juju_timings

        # Each clone shares a reference to _added_models to ensure we track any
        # added models regardless of the ModelClient that adds them.
        result._added_models = self._added_models
        return result

    def track_model(self, client):
        # Keep a reference to `client` for the lifetime of this backend (or
        # until it's untracked).
        self._added_models.append(client)

    def untrack_model(self, client):
        """Remove `client` from tracking. Silently fails if not present."""
        # No longer need to track this client for whatever reason.
        try:
            self._added_models.remove(client)
        except ValueError:
            log.debug(
                'Attempted to remove client "{}" that was not tracked.'.format(
                    client.env.environment))
            pass

    @property
    def version(self):
        return self._version

    @property
    def full_path(self):
        return self._full_path

    @property
    def juju_name(self):
        return os.path.basename(self._full_path)

    @property
    def added_models(self):
        # Return a copy of the list so any modifications don't trip callees up.
        return list(self._added_models)

    def _get_attr_tuple(self):
        return (self._version, self._full_path, self.feature_flags,
                self.debug, self.juju_timings)

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return self._get_attr_tuple() == other._get_attr_tuple()

    def shell_environ(self, used_feature_flags, juju_home):
        """Generate a suitable shell environment.

        Juju's directory must be in the PATH to support plugins.
        """
        env = dict(os.environ)
        if self.full_path is not None:
            env['PATH'] = '{}{}{}'.format(os.path.dirname(self.full_path),
                                          os.pathsep, env['PATH'])
        flags = self.feature_flags.intersection(used_feature_flags)
        feature_flag_string = env.get(JUJU_DEV_FEATURE_FLAGS, '')
        if feature_flag_string != '':
            flags.update(feature_flag_string.split(','))
        if flags:
            env[JUJU_DEV_FEATURE_FLAGS] = ','.join(sorted(flags))
        env['JUJU_DATA'] = juju_home
        return env

    def full_args(self, command, args, model, timeout):
        if model is not None:
            e_arg = (self._model_flag, model)
        else:
            e_arg = ()
        if timeout is None:
            prefix = ()
        else:
            prefix = get_timeout_prefix(timeout, self._timeout_path)
        logging = '--debug' if self.debug else '--show-log'

        # If args is a string, make it a tuple. This makes writing commands
        # with one argument a bit nicer.
        if isinstance(args, argtype):
            args = (args,)
        # we split the command here so that the caller can control where the -m
        # model flag goes.  Everything in the command string is put before the
        # -m flag.
        command = command.split()
        return (prefix + (self.juju_name, logging,) + tuple(command) + e_arg +
                args)

    def juju(self, command, args, used_feature_flags,
             juju_home, model=None, check=True, timeout=None, extra_env=None,
             suppress_err=False):
        """Run a command under juju for the current environment.

        :return: Tuple rval, CommandTime rval being the commands exit code and
          a CommandTime object used for storing command timing data.
        """
        args = self.full_args(command, args, model, timeout)
        log.info(' '.join(args))
        env = self.shell_environ(used_feature_flags, juju_home)
        if extra_env is not None:
            env.update(extra_env)
        if check:
            call_func = subprocess.check_call
        else:
            call_func = subprocess.call
        # Mutate os.environ instead of supplying env parameter so Windows can
        # search env['PATH']
        stderr = subprocess.PIPE if suppress_err else None
        # Keep track of commands and how long the take.
        command_time = CommandTime(command, args, env)
        with scoped_environ(env):
            log.debug('Running juju with env: {}'.format(env))
            with self._check_timeouts():
                rval = call_func(args, stderr=stderr)
        self.juju_timings.append(command_time)
        return rval, command_time

    def expect(self, command, args, used_feature_flags, juju_home, model=None,
               timeout=None, extra_env=None):
        args = self.full_args(command, args, model, timeout)
        log.info(' '.join(args))
        env = self.shell_environ(used_feature_flags, juju_home)
        if extra_env is not None:
            env.update(extra_env)
        # pexpect.spawn expects a string. This is better than trying to extract
        # command + args from the returned tuple (as there could be an intial
        # timing command tacked on).
        command_string = ' '.join(quote(a) for a in args)
        with scoped_environ(env):
            return pexpect.spawn(command_string)

    @contextmanager
    def juju_async(self, command, args, used_feature_flags,
                   juju_home, model=None, timeout=None):
        full_args = self.full_args(command, args, model, timeout)
        log.info(' '.join(args))
        env = self.shell_environ(used_feature_flags, juju_home)
        # Mutate os.environ instead of supplying env parameter so Windows can
        # search env['PATH']
        with scoped_environ(env):
            with self._check_timeouts():
                proc = subprocess.Popen(full_args)
        yield proc
        retcode = proc.wait()
        if retcode != 0:
            raise subprocess.CalledProcessError(retcode, full_args)

    def get_juju_output(self, command, args, used_feature_flags, juju_home,
                        model=None, timeout=None, user_name=None,
                        merge_stderr=False):
        args = self.full_args(command, args, model, timeout)
        env = self.shell_environ(used_feature_flags, juju_home)
        log.debug(args)
        # Mutate os.environ instead of supplying env parameter so
        # Windows can search env['PATH']
        with scoped_environ(env):
            proc = subprocess.Popen(
                args, stdout=subprocess.PIPE, stdin=subprocess.PIPE,
                stderr=subprocess.STDOUT if merge_stderr else subprocess.PIPE)
            with self._check_timeouts():
                sub_output, sub_error = proc.communicate()
            log.debug(sub_output)
            if proc.returncode != 0:
                log.debug(sub_error)
                e = subprocess.CalledProcessError(
                    proc.returncode, args, sub_output)
                e.stderr = sub_error
                if sub_error and (
                    b'Unable to connect to environment' in sub_error or
                        b'MissingOrIncorrectVersionHeader' in sub_error or
                        b'307: Temporary Redirect' in sub_error):
                    raise CannotConnectEnv(e)
                raise e
        return sub_output

    def get_active_model(self, juju_data_dir):
        """Determine the active model in a juju data dir."""
        try:
            current = json.loads(self.get_juju_output(
                'models', ('--format', 'json'), set(),
                juju_data_dir, model=None).decode('ascii'))
        except subprocess.CalledProcessError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))
        try:
            return current['current-model']
        except KeyError:
            raise NoActiveModel('No active model for {}'.format(juju_data_dir))

    def get_active_controller(self, juju_data_dir):
        """Determine the active controller in a juju data dir."""
        try:
            current = json.loads(self.get_juju_output(
                'controllers', ('--format', 'json'), set(),
                juju_data_dir, model=None).decode('ascii'))
        except subprocess.CalledProcessError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))
        try:
            return current['current-controller']
        except KeyError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))

    def get_active_user(self, juju_data_dir, controller):
        """Determine the active user for a controller."""
        try:
            current = json.loads(self.get_juju_output(
                'controllers', ('--format', 'json'), set(),
                juju_data_dir, model=None).decode('ascii'))
        except subprocess.CalledProcessError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))
        return current['controllers'][controller]['user']

    def pause(self, seconds):
        pause(seconds)
