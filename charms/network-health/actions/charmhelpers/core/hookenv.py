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

"Interactions with the Juju environment"
# Copyright 2013 Canonical Ltd.
#
# Authors:
#  Charm Helpers Developers <juju@lists.ubuntu.com>

from __future__ import print_function
import copy
from distutils.version import LooseVersion
from functools import wraps
import glob
import os
import json
import yaml
import subprocess
import sys
import errno
import tempfile
from subprocess import CalledProcessError

import six
if not six.PY3:
    from UserDict import UserDict
else:
    from collections import UserDict

CRITICAL = "CRITICAL"
ERROR = "ERROR"
WARNING = "WARNING"
INFO = "INFO"
DEBUG = "DEBUG"
MARKER = object()

cache = {}


def cached(func):
    """Cache return values for multiple executions of func + args

    For example::

        @cached
        def unit_get(attribute):
            pass

        unit_get('test')

    will cache the result of unit_get + 'test' for future calls.
    """
    @wraps(func)
    def wrapper(*args, **kwargs):
        global cache
        key = str((func, args, kwargs))
        try:
            return cache[key]
        except KeyError:
            pass  # Drop out of the exception handler scope.
        res = func(*args, **kwargs)
        cache[key] = res
        return res
    wrapper._wrapped = func
    return wrapper


def flush(key):
    """Flushes any entries from function cache where the
    key is found in the function+args """
    flush_list = []
    for item in cache:
        if key in item:
            flush_list.append(item)
    for item in flush_list:
        del cache[item]


def log(message, level=None):
    """Write a message to the juju log"""
    command = ['juju-log']
    if level:
        command += ['-l', level]
    if not isinstance(message, six.string_types):
        message = repr(message)
    command += [message]
    # Missing juju-log should not cause failures in unit tests
    # Send log output to stderr
    try:
        subprocess.call(command)
    except OSError as e:
        if e.errno == errno.ENOENT:
            if level:
                message = "{}: {}".format(level, message)
            message = "juju-log: {}".format(message)
            print(message, file=sys.stderr)
        else:
            raise


class Serializable(UserDict):
    """Wrapper, an object that can be serialized to yaml or json"""

    def __init__(self, obj):
        # wrap the object
        UserDict.__init__(self)
        self.data = obj

    def __getattr__(self, attr):
        # See if this object has attribute.
        if attr in ("json", "yaml", "data"):
            return self.__dict__[attr]
        # Check for attribute in wrapped object.
        got = getattr(self.data, attr, MARKER)
        if got is not MARKER:
            return got
        # Proxy to the wrapped object via dict interface.
        try:
            return self.data[attr]
        except KeyError:
            raise AttributeError(attr)

    def __getstate__(self):
        # Pickle as a standard dictionary.
        return self.data

    def __setstate__(self, state):
        # Unpickle into our wrapper.
        self.data = state

    def json(self):
        """Serialize the object to json"""
        return json.dumps(self.data)

    def yaml(self):
        """Serialize the object to yaml"""
        return yaml.dump(self.data)


def execution_environment():
    """A convenient bundling of the current execution context"""
    context = {}
    context['conf'] = config()
    if relation_id():
        context['reltype'] = relation_type()
        context['relid'] = relation_id()
        context['rel'] = relation_get()
    context['unit'] = local_unit()
    context['rels'] = relations()
    context['env'] = os.environ
    return context


def in_relation_hook():
    """Determine whether we're running in a relation hook"""
    return 'JUJU_RELATION' in os.environ


def relation_type():
    """The scope for the current relation hook"""
    return os.environ.get('JUJU_RELATION', None)


@cached
def relation_id(relation_name=None, service_or_unit=None):
    """The relation ID for the current or a specified relation"""
    if not relation_name and not service_or_unit:
        return os.environ.get('JUJU_RELATION_ID', None)
    elif relation_name and service_or_unit:
        service_name = service_or_unit.split('/')[0]
        for relid in relation_ids(relation_name):
            remote_service = remote_service_name(relid)
            if remote_service == service_name:
                return relid
    else:
        raise ValueError('Must specify neither or both of relation_name and service_or_unit')


def local_unit():
    """Local unit ID"""
    return os.environ['JUJU_UNIT_NAME']


def remote_unit():
    """The remote unit for the current relation hook"""
    return os.environ.get('JUJU_REMOTE_UNIT', None)


def service_name():
    """The name service group this unit belongs to"""
    return local_unit().split('/')[0]


@cached
def remote_service_name(relid=None):
    """The remote service name for a given relation-id (or the current relation)"""
    if relid is None:
        unit = remote_unit()
    else:
        units = related_units(relid)
        unit = units[0] if units else None
    return unit.split('/')[0] if unit else None


def hook_name():
    """The name of the currently executing hook"""
    return os.environ.get('JUJU_HOOK_NAME', os.path.basename(sys.argv[0]))


class Config(dict):
    """A dictionary representation of the charm's config.yaml, with some
    extra features:

    - See which values in the dictionary have changed since the previous hook.
    - For values that have changed, see what the previous value was.
    - Store arbitrary data for use in a later hook.

    NOTE: Do not instantiate this object directly - instead call
    ``hookenv.config()``, which will return an instance of :class:`Config`.

    Example usage::

        >>> # inside a hook
        >>> from charmhelpers.core import hookenv
        >>> config = hookenv.config()
        >>> config['foo']
        'bar'
        >>> # store a new key/value for later use
        >>> config['mykey'] = 'myval'


        >>> # user runs `juju set mycharm foo=baz`
        >>> # now we're inside subsequent config-changed hook
        >>> config = hookenv.config()
        >>> config['foo']
        'baz'
        >>> # test to see if this val has changed since last hook
        >>> config.changed('foo')
        True
        >>> # what was the previous value?
        >>> config.previous('foo')
        'bar'
        >>> # keys/values that we add are preserved across hooks
        >>> config['mykey']
        'myval'

    """
    CONFIG_FILE_NAME = '.juju-persistent-config'

    def __init__(self, *args, **kw):
        super(Config, self).__init__(*args, **kw)
        self.implicit_save = True
        self._prev_dict = None
        self.path = os.path.join(charm_dir(), Config.CONFIG_FILE_NAME)
        if os.path.exists(self.path):
            self.load_previous()
        atexit(self._implicit_save)

    def load_previous(self, path=None):
        """Load previous copy of config from disk.

        In normal usage you don't need to call this method directly - it
        is called automatically at object initialization.

        :param path:

            File path from which to load the previous config. If `None`,
            config is loaded from the default location. If `path` is
            specified, subsequent `save()` calls will write to the same
            path.

        """
        self.path = path or self.path
        with open(self.path) as f:
            self._prev_dict = json.load(f)
        for k, v in copy.deepcopy(self._prev_dict).items():
            if k not in self:
                self[k] = v

    def changed(self, key):
        """Return True if the current value for this key is different from
        the previous value.

        """
        if self._prev_dict is None:
            return True
        return self.previous(key) != self.get(key)

    def previous(self, key):
        """Return previous value for this key, or None if there
        is no previous value.

        """
        if self._prev_dict:
            return self._prev_dict.get(key)
        return None

    def save(self):
        """Save this config to disk.

        If the charm is using the :mod:`Services Framework <services.base>`
        or :meth:'@hook <Hooks.hook>' decorator, this
        is called automatically at the end of successful hook execution.
        Otherwise, it should be called directly by user code.

        To disable automatic saves, set ``implicit_save=False`` on this
        instance.

        """
        with open(self.path, 'w') as f:
            json.dump(self, f)

    def _implicit_save(self):
        if self.implicit_save:
            self.save()


@cached
def config(scope=None):
    """Juju charm configuration"""
    config_cmd_line = ['config-get']
    if scope is not None:
        config_cmd_line.append(scope)
    else:
        config_cmd_line.append('--all')
    config_cmd_line.append('--format=json')
    try:
        config_data = json.loads(
            subprocess.check_output(config_cmd_line).decode('UTF-8'))
        if scope is not None:
            return config_data
        return Config(config_data)
    except ValueError:
        return None


@cached
def relation_get(attribute=None, unit=None, rid=None):
    """Get relation information"""
    _args = ['relation-get', '--format=json']
    if rid:
        _args.append('-r')
        _args.append(rid)
    _args.append(attribute or '-')
    if unit:
        _args.append(unit)
    try:
        return json.loads(subprocess.check_output(_args).decode('UTF-8'))
    except ValueError:
        return None
    except CalledProcessError as e:
        if e.returncode == 2:
            return None
        raise


def relation_set(relation_id=None, relation_settings=None, **kwargs):
    """Set relation information for the current unit"""
    relation_settings = relation_settings if relation_settings else {}
    relation_cmd_line = ['relation-set']
    accepts_file = "--file" in subprocess.check_output(
        relation_cmd_line + ["--help"], universal_newlines=True)
    if relation_id is not None:
        relation_cmd_line.extend(('-r', relation_id))
    settings = relation_settings.copy()
    settings.update(kwargs)
    for key, value in settings.items():
        # Force value to be a string: it always should, but some call
        # sites pass in things like dicts or numbers.
        if value is not None:
            settings[key] = "{}".format(value)
    if accepts_file:
        # --file was introduced in Juju 1.23.2. Use it by default if
        # available, since otherwise we'll break if the relation data is
        # too big. Ideally we should tell relation-set to read the data from
        # stdin, but that feature is broken in 1.23.2: Bug #1454678.
        with tempfile.NamedTemporaryFile(delete=False) as settings_file:
            settings_file.write(yaml.safe_dump(settings).encode("utf-8"))
        subprocess.check_call(
            relation_cmd_line + ["--file", settings_file.name])
        os.remove(settings_file.name)
    else:
        for key, value in settings.items():
            if value is None:
                relation_cmd_line.append('{}='.format(key))
            else:
                relation_cmd_line.append('{}={}'.format(key, value))
        subprocess.check_call(relation_cmd_line)
    # Flush cache of any relation-gets for local unit
    flush(local_unit())


def relation_clear(r_id=None):
    ''' Clears any relation data already set on relation r_id '''
    settings = relation_get(rid=r_id,
                            unit=local_unit())
    for setting in settings:
        if setting not in ['public-address', 'private-address']:
            settings[setting] = None
    relation_set(relation_id=r_id,
                 **settings)


@cached
def relation_ids(reltype=None):
    """A list of relation_ids"""
    reltype = reltype or relation_type()
    relid_cmd_line = ['relation-ids', '--format=json']
    if reltype is not None:
        relid_cmd_line.append(reltype)
        return json.loads(
            subprocess.check_output(relid_cmd_line).decode('UTF-8')) or []
    return []


@cached
def related_units(relid=None):
    """A list of related units"""
    relid = relid or relation_id()
    units_cmd_line = ['relation-list', '--format=json']
    if relid is not None:
        units_cmd_line.extend(('-r', relid))
    return json.loads(
        subprocess.check_output(units_cmd_line).decode('UTF-8')) or []


@cached
def relation_for_unit(unit=None, rid=None):
    """Get the json represenation of a unit's relation"""
    unit = unit or remote_unit()
    relation = relation_get(unit=unit, rid=rid)
    for key in relation:
        if key.endswith('-list'):
            relation[key] = relation[key].split()
    relation['__unit__'] = unit
    return relation


@cached
def relations_for_id(relid=None):
    """Get relations of a specific relation ID"""
    relation_data = []
    relid = relid or relation_ids()
    for unit in related_units(relid):
        unit_data = relation_for_unit(unit, relid)
        unit_data['__relid__'] = relid
        relation_data.append(unit_data)
    return relation_data


@cached
def relations_of_type(reltype=None):
    """Get relations of a specific type"""
    relation_data = []
    reltype = reltype or relation_type()
    for relid in relation_ids(reltype):
        for relation in relations_for_id(relid):
            relation['__relid__'] = relid
            relation_data.append(relation)
    return relation_data


@cached
def metadata():
    """Get the current charm metadata.yaml contents as a python object"""
    with open(os.path.join(charm_dir(), 'metadata.yaml')) as md:
        return yaml.safe_load(md)


@cached
def relation_types():
    """Get a list of relation types supported by this charm"""
    rel_types = []
    md = metadata()
    for key in ('provides', 'requires', 'peers'):
        section = md.get(key)
        if section:
            rel_types.extend(section.keys())
    return rel_types


@cached
def peer_relation_id():
    '''Get the peers relation id if a peers relation has been joined, else None.'''
    md = metadata()
    section = md.get('peers')
    if section:
        for key in section:
            relids = relation_ids(key)
            if relids:
                return relids[0]
    return None


@cached
def relation_to_interface(relation_name):
    """
    Given the name of a relation, return the interface that relation uses.

    :returns: The interface name, or ``None``.
    """
    return relation_to_role_and_interface(relation_name)[1]


@cached
def relation_to_role_and_interface(relation_name):
    """
    Given the name of a relation, return the role and the name of the interface
    that relation uses (where role is one of ``provides``, ``requires``, or ``peers``).

    :returns: A tuple containing ``(role, interface)``, or ``(None, None)``.
    """
    _metadata = metadata()
    for role in ('provides', 'requires', 'peers'):
        interface = _metadata.get(role, {}).get(relation_name, {}).get('interface')
        if interface:
            return role, interface
    return None, None


@cached
def role_and_interface_to_relations(role, interface_name):
    """
    Given a role and interface name, return a list of relation names for the
    current charm that use that interface under that role (where role is one
    of ``provides``, ``requires``, or ``peers``).

    :returns: A list of relation names.
    """
    _metadata = metadata()
    results = []
    for relation_name, relation in _metadata.get(role, {}).items():
        if relation['interface'] == interface_name:
            results.append(relation_name)
    return results


@cached
def interface_to_relations(interface_name):
    """
    Given an interface, return a list of relation names for the current
    charm that use that interface.

    :returns: A list of relation names.
    """
    results = []
    for role in ('provides', 'requires', 'peers'):
        results.extend(role_and_interface_to_relations(role, interface_name))
    return results


@cached
def charm_name():
    """Get the name of the current charm as is specified on metadata.yaml"""
    return metadata().get('name')


@cached
def relations():
    """Get a nested dictionary of relation data for all related units"""
    rels = {}
    for reltype in relation_types():
        relids = {}
        for relid in relation_ids(reltype):
            units = {local_unit(): relation_get(unit=local_unit(), rid=relid)}
            for unit in related_units(relid):
                reldata = relation_get(unit=unit, rid=relid)
                units[unit] = reldata
            relids[relid] = units
        rels[reltype] = relids
    return rels


@cached
def is_relation_made(relation, keys='private-address'):
    '''
    Determine whether a relation is established by checking for
    presence of key(s).  If a list of keys is provided, they
    must all be present for the relation to be identified as made
    '''
    if isinstance(keys, str):
        keys = [keys]
    for r_id in relation_ids(relation):
        for unit in related_units(r_id):
            context = {}
            for k in keys:
                context[k] = relation_get(k, rid=r_id,
                                          unit=unit)
            if None not in context.values():
                return True
    return False


def open_port(port, protocol="TCP"):
    """Open a service network port"""
    _args = ['open-port']
    _args.append('{}/{}'.format(port, protocol))
    subprocess.check_call(_args)


def close_port(port, protocol="TCP"):
    """Close a service network port"""
    _args = ['close-port']
    _args.append('{}/{}'.format(port, protocol))
    subprocess.check_call(_args)


@cached
def unit_get(attribute):
    """Get the unit ID for the remote unit"""
    _args = ['unit-get', '--format=json', attribute]
    try:
        return json.loads(subprocess.check_output(_args).decode('UTF-8'))
    except ValueError:
        return None


def unit_public_ip():
    """Get this unit's public IP address"""
    return unit_get('public-address')


def unit_private_ip():
    """Get this unit's private IP address"""
    return unit_get('private-address')


@cached
def storage_get(attribute=None, storage_id=None):
    """Get storage attributes"""
    _args = ['storage-get', '--format=json']
    if storage_id:
        _args.extend(('-s', storage_id))
    if attribute:
        _args.append(attribute)
    try:
        return json.loads(subprocess.check_output(_args).decode('UTF-8'))
    except ValueError:
        return None


@cached
def storage_list(storage_name=None):
    """List the storage IDs for the unit"""
    _args = ['storage-list', '--format=json']
    if storage_name:
        _args.append(storage_name)
    try:
        return json.loads(subprocess.check_output(_args).decode('UTF-8'))
    except ValueError:
        return None
    except OSError as e:
        import errno
        if e.errno == errno.ENOENT:
            # storage-list does not exist
            return []
        raise


class UnregisteredHookError(Exception):
    """Raised when an undefined hook is called"""
    pass


class Hooks(object):
    """A convenient handler for hook functions.

    Example::

        hooks = Hooks()

        # register a hook, taking its name from the function name
        @hooks.hook()
        def install():
            pass  # your code here

        # register a hook, providing a custom hook name
        @hooks.hook("config-changed")
        def config_changed():
            pass  # your code here

        if __name__ == "__main__":
            # execute a hook based on the name the program is called by
            hooks.execute(sys.argv)
    """

    def __init__(self, config_save=None):
        super(Hooks, self).__init__()
        self._hooks = {}

        # For unknown reasons, we allow the Hooks constructor to override
        # config().implicit_save.
        if config_save is not None:
            config().implicit_save = config_save

    def register(self, name, function):
        """Register a hook"""
        self._hooks[name] = function

    def execute(self, args):
        """Execute a registered hook based on args[0]"""
        _run_atstart()
        hook_name = os.path.basename(args[0])
        if hook_name in self._hooks:
            try:
                self._hooks[hook_name]()
            except SystemExit as x:
                if x.code is None or x.code == 0:
                    _run_atexit()
                raise
            _run_atexit()
        else:
            raise UnregisteredHookError(hook_name)

    def hook(self, *hook_names):
        """Decorator, registering them as hooks"""
        def wrapper(decorated):
            for hook_name in hook_names:
                self.register(hook_name, decorated)
            else:
                self.register(decorated.__name__, decorated)
                if '_' in decorated.__name__:
                    self.register(
                        decorated.__name__.replace('_', '-'), decorated)
            return decorated
        return wrapper


def charm_dir():
    """Return the root directory of the current charm"""
    return os.environ.get('CHARM_DIR')


@cached
def action_get(key=None):
    """Gets the value of an action parameter, or all key/value param pairs"""
    cmd = ['action-get']
    if key is not None:
        cmd.append(key)
    cmd.append('--format=json')
    action_data = json.loads(subprocess.check_output(cmd).decode('UTF-8'))
    return action_data


def action_set(values):
    """Sets the values to be returned after the action finishes"""
    cmd = ['action-set']
    for k, v in list(values.items()):
        cmd.append('{}={}'.format(k, v))
    subprocess.check_call(cmd)


def action_fail(message):
    """Sets the action status to failed and sets the error message.

    The results set by action_set are preserved."""
    subprocess.check_call(['action-fail', message])


def action_name():
    """Get the name of the currently executing action."""
    return os.environ.get('JUJU_ACTION_NAME')


def action_uuid():
    """Get the UUID of the currently executing action."""
    return os.environ.get('JUJU_ACTION_UUID')


def action_tag():
    """Get the tag for the currently executing action."""
    return os.environ.get('JUJU_ACTION_TAG')


def status_set(workload_state, message):
    """Set the workload state with a message

    Use status-set to set the workload state with a message which is visible
    to the user via juju status. If the status-set command is not found then
    assume this is juju < 1.23 and juju-log the message unstead.

    workload_state -- valid juju workload state.
    message        -- status update message
    """
    valid_states = ['maintenance', 'blocked', 'waiting', 'active']
    if workload_state not in valid_states:
        raise ValueError(
            '{!r} is not a valid workload state'.format(workload_state)
        )
    cmd = ['status-set', workload_state, message]
    try:
        ret = subprocess.call(cmd)
        if ret == 0:
            return
    except OSError as e:
        if e.errno != errno.ENOENT:
            raise
    log_message = 'status-set failed: {} {}'.format(workload_state,
                                                    message)
    log(log_message, level='INFO')


def status_get():
    """Retrieve the previously set juju workload state and message

    If the status-get command is not found then assume this is juju < 1.23 and
    return 'unknown', ""

    """
    cmd = ['status-get', "--format=json", "--include-data"]
    try:
        raw_status = subprocess.check_output(cmd)
    except OSError as e:
        if e.errno == errno.ENOENT:
            return ('unknown', "")
        else:
            raise
    else:
        status = json.loads(raw_status.decode("UTF-8"))
        return (status["status"], status["message"])


def translate_exc(from_exc, to_exc):
    def inner_translate_exc1(f):
        @wraps(f)
        def inner_translate_exc2(*args, **kwargs):
            try:
                return f(*args, **kwargs)
            except from_exc:
                raise to_exc

        return inner_translate_exc2

    return inner_translate_exc1


def application_version_set(version):
    """Charm authors may trigger this command from any hook to output what
    version of the application is running. This could be a package version,
    for instance postgres version 9.5. It could also be a build number or
    version control revision identifier, for instance git sha 6fb7ba68. """

    cmd = ['application-version-set']
    cmd.append(version)
    try:
        subprocess.check_call(cmd)
    except OSError:
        log("Application Version: {}".format(version))


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def is_leader():
    """Does the current unit hold the juju leadership

    Uses juju to determine whether the current unit is the leader of its peers
    """
    cmd = ['is-leader', '--format=json']
    return json.loads(subprocess.check_output(cmd).decode('UTF-8'))


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def leader_get(attribute=None):
    """Juju leader get value(s)"""
    cmd = ['leader-get', '--format=json'] + [attribute or '-']
    return json.loads(subprocess.check_output(cmd).decode('UTF-8'))


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def leader_set(settings=None, **kwargs):
    """Juju leader set value(s)"""
    # Don't log secrets.
    # log("Juju leader-set '%s'" % (settings), level=DEBUG)
    cmd = ['leader-set']
    settings = settings or {}
    settings.update(kwargs)
    for k, v in settings.items():
        if v is None:
            cmd.append('{}='.format(k))
        else:
            cmd.append('{}={}'.format(k, v))
    subprocess.check_call(cmd)


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def payload_register(ptype, klass, pid):
    """ is used while a hook is running to let Juju know that a
        payload has been started."""
    cmd = ['payload-register']
    for x in [ptype, klass, pid]:
        cmd.append(x)
    subprocess.check_call(cmd)


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def payload_unregister(klass, pid):
    """ is used while a hook is running to let Juju know
    that a payload has been manually stopped. The <class> and <id> provided
    must match a payload that has been previously registered with juju using
    payload-register."""
    cmd = ['payload-unregister']
    for x in [klass, pid]:
        cmd.append(x)
    subprocess.check_call(cmd)


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def payload_status_set(klass, pid, status):
    """is used to update the current status of a registered payload.
    The <class> and <id> provided must match a payload that has been previously
    registered with juju using payload-register. The <status> must be one of the
    follow: starting, started, stopping, stopped"""
    cmd = ['payload-status-set']
    for x in [klass, pid, status]:
        cmd.append(x)
    subprocess.check_call(cmd)


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def resource_get(name):
    """used to fetch the resource path of the given name.

    <name> must match a name of defined resource in metadata.yaml

    returns either a path or False if resource not available
    """
    if not name:
        return False

    cmd = ['resource-get', name]
    try:
        return subprocess.check_output(cmd).decode('UTF-8')
    except subprocess.CalledProcessError:
        return False


@cached
def juju_version():
    """Full version string (eg. '1.23.3.1-trusty-amd64')"""
    # Per https://bugs.launchpad.net/juju-core/+bug/1455368/comments/1
    jujud = glob.glob('/var/lib/juju/tools/machine-*/jujud')[0]
    return subprocess.check_output([jujud, 'version'],
                                   universal_newlines=True).strip()


@cached
def has_juju_version(minimum_version):
    """Return True if the Juju version is at least the provided version"""
    return LooseVersion(juju_version()) >= LooseVersion(minimum_version)


_atexit = []
_atstart = []


def atstart(callback, *args, **kwargs):
    '''Schedule a callback to run before the main hook.

    Callbacks are run in the order they were added.

    This is useful for modules and classes to perform initialization
    and inject behavior. In particular:

        - Run common code before all of your hooks, such as logging
          the hook name or interesting relation data.
        - Defer object or module initialization that requires a hook
          context until we know there actually is a hook context,
          making testing easier.
        - Rather than requiring charm authors to include boilerplate to
          invoke your helper's behavior, have it run automatically if
          your object is instantiated or module imported.

    This is not at all useful after your hook framework as been launched.
    '''
    global _atstart
    _atstart.append((callback, args, kwargs))


def atexit(callback, *args, **kwargs):
    '''Schedule a callback to run on successful hook completion.

    Callbacks are run in the reverse order that they were added.'''
    _atexit.append((callback, args, kwargs))


def _run_atstart():
    '''Hook frameworks must invoke this before running the main hook body.'''
    global _atstart
    for callback, args, kwargs in _atstart:
        callback(*args, **kwargs)
    del _atstart[:]


def _run_atexit():
    '''Hook frameworks must invoke this after the main hook body has
    successfully completed. Do not invoke it if the hook fails.'''
    global _atexit
    for callback, args, kwargs in reversed(_atexit):
        callback(*args, **kwargs)
    del _atexit[:]


@translate_exc(from_exc=OSError, to_exc=NotImplementedError)
def network_get_primary_address(binding):
    '''
    Retrieve the primary network address for a named binding

    :param binding: string. The name of a relation of extra-binding
    :return: string. The primary IP address for the named binding
    :raise: NotImplementedError if run on Juju < 2.0
    '''
    cmd = ['network-get', '--primary-address', binding]
    return subprocess.check_output(cmd).decode('UTF-8').strip()
