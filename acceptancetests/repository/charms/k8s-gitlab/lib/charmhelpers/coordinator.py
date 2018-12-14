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

'''
The coordinator module allows you to use Juju's leadership feature to
coordinate operations between units of a service.

Behavior is defined in subclasses of coordinator.BaseCoordinator.
One implementation is provided (coordinator.Serial), which allows an
operation to be run on a single unit at a time, on a first come, first
served basis. You can trivially define more complex behavior by
subclassing BaseCoordinator or Serial.

:author: Stuart Bishop <stuart.bishop@canonical.com>


Services Framework Usage
========================

Ensure a peers relation is defined in metadata.yaml. Instantiate a
BaseCoordinator subclass before invoking ServiceManager.manage().
Ensure that ServiceManager.manage() is wired up to the leader-elected,
leader-settings-changed, peers relation-changed and peers
relation-departed hooks in addition to any other hooks you need, or your
service will deadlock.

Ensure calls to acquire() are guarded, so that locks are only requested
when they are really needed (and thus hooks only triggered when necessary).
Failing to do this and calling acquire() unconditionally will put your unit
into a hook loop. Calls to granted() do not need to be guarded.

For example::

    from charmhelpers.core import hookenv, services
    from charmhelpers import coordinator

    def maybe_restart(servicename):
        serial = coordinator.Serial()
        if needs_restart():
            serial.acquire('restart')
        if serial.granted('restart'):
            hookenv.service_restart(servicename)

    services = [dict(service='servicename',
                     data_ready=[maybe_restart])]

    if __name__ == '__main__':
        _ = coordinator.Serial()  # Must instantiate before manager.manage()
        manager = services.ServiceManager(services)
        manager.manage()


You can implement a similar pattern using a decorator. If the lock has
not been granted, an attempt to acquire() it will be made if the guard
function returns True. If the lock has been granted, the decorated function
is run as normal::

    from charmhelpers.core import hookenv, services
    from charmhelpers import coordinator

    serial = coordinator.Serial()  # Global, instatiated on module import.

    def needs_restart():
        [ ... Introspect state. Return True if restart is needed ... ]

    @serial.require('restart', needs_restart)
    def maybe_restart(servicename):
        hookenv.service_restart(servicename)

    services = [dict(service='servicename',
                     data_ready=[maybe_restart])]

    if __name__ == '__main__':
        manager = services.ServiceManager(services)
        manager.manage()


Traditional Usage
=================

Ensure a peers relation is defined in metadata.yaml.

If you are using charmhelpers.core.hookenv.Hooks, ensure that a
BaseCoordinator subclass is instantiated before calling Hooks.execute.

If you are not using charmhelpers.core.hookenv.Hooks, ensure
that a BaseCoordinator subclass is instantiated and its handle()
method called at the start of all your hooks.

For example::

    import sys
    from charmhelpers.core import hookenv
    from charmhelpers import coordinator

    hooks = hookenv.Hooks()

    def maybe_restart():
        serial = coordinator.Serial()
        if serial.granted('restart'):
            hookenv.service_restart('myservice')

    @hooks.hook
    def config_changed():
        update_config()
        serial = coordinator.Serial()
        if needs_restart():
            serial.acquire('restart'):
            maybe_restart()

    # Cluster hooks must be wired up.
    @hooks.hook('cluster-relation-changed', 'cluster-relation-departed')
    def cluster_relation_changed():
        maybe_restart()

    # Leader hooks must be wired up.
    @hooks.hook('leader-elected', 'leader-settings-changed')
    def leader_settings_changed():
        maybe_restart()

    [ ... repeat for *all* other hooks you are using ... ]

    if __name__ == '__main__':
        _ = coordinator.Serial()  # Must instantiate before execute()
        hooks.execute(sys.argv)


You can also use the require decorator. If the lock has not been granted,
an attempt to acquire() it will be made if the guard function returns True.
If the lock has been granted, the decorated function is run as normal::

    from charmhelpers.core import hookenv

    hooks = hookenv.Hooks()
    serial = coordinator.Serial()  # Must instantiate before execute()

    @require('restart', needs_restart)
    def maybe_restart():
        hookenv.service_restart('myservice')

    @hooks.hook('install', 'config-changed', 'upgrade-charm',
                # Peers and leader hooks must be wired up.
                'cluster-relation-changed', 'cluster-relation-departed',
                'leader-elected', 'leader-settings-changed')
    def default_hook():
        [...]
        maybe_restart()

    if __name__ == '__main__':
        hooks.execute()


Details
=======

A simple API is provided similar to traditional locking APIs. A lock
may be requested using the acquire() method, and the granted() method
may be used do to check if a lock previously requested by acquire() has
been granted. It doesn't matter how many times acquire() is called in a
hook.

Locks are released at the end of the hook they are acquired in. This may
be the current hook if the unit is leader and the lock is free. It is
more likely a future hook (probably leader-settings-changed, possibly
the peers relation-changed or departed hook, potentially any hook).

Whenever a charm needs to perform a coordinated action it will acquire()
the lock and perform the action immediately if acquisition is
successful. It will also need to perform the same action in every other
hook if the lock has been granted.


Grubby Details
--------------

Why do you need to be able to perform the same action in every hook?
If the unit is the leader, then it may be able to grant its own lock
and perform the action immediately in the source hook. If the unit is
the leader and cannot immediately grant the lock, then its only
guaranteed chance of acquiring the lock is in the peers relation-joined,
relation-changed or peers relation-departed hooks when another unit has
released it (the only channel to communicate to the leader is the peers
relation). If the unit is not the leader, then it is unlikely the lock
is granted in the source hook (a previous hook must have also made the
request for this to happen). A non-leader is notified about the lock via
leader settings. These changes may be visible in any hook, even before
the leader-settings-changed hook has been invoked. Or the requesting
unit may be promoted to leader after making a request, in which case the
lock may be granted in leader-elected or in a future peers
relation-changed or relation-departed hook.

This could be simpler if leader-settings-changed was invoked on the
leader. We could then never grant locks except in
leader-settings-changed hooks giving one place for the operation to be
performed. Unfortunately this is not the case with Juju 1.23 leadership.

But of course, this doesn't really matter to most people as most people
seem to prefer the Services Framework or similar reset-the-world
approaches, rather than the twisty maze of attempting to deduce what
should be done based on what hook happens to be running (which always
seems to evolve into reset-the-world anyway when the charm grows beyond
the trivial).

I chose not to implement a callback model, where a callback was passed
to acquire to be executed when the lock is granted, because the callback
may become invalid between making the request and the lock being granted
due to an upgrade-charm being run in the interim. And it would create
restrictions, such no lambdas, callback defined at the top level of a
module, etc. Still, we could implement it on top of what is here, eg.
by adding a defer decorator that stores a pickle of itself to disk and
have BaseCoordinator unpickle and execute them when the locks are granted.
'''
from datetime import datetime
from functools import wraps
import json
import os.path

from six import with_metaclass

from charmhelpers.core import hookenv


# We make BaseCoordinator and subclasses singletons, so that if we
# need to spill to local storage then only a single instance does so,
# rather than having multiple instances stomp over each other.
class Singleton(type):
    _instances = {}

    def __call__(cls, *args, **kwargs):
        if cls not in cls._instances:
            cls._instances[cls] = super(Singleton, cls).__call__(*args,
                                                                 **kwargs)
        return cls._instances[cls]


class BaseCoordinator(with_metaclass(Singleton, object)):
    relid = None  # Peer relation-id, set by __init__
    relname = None

    grants = None  # self.grants[unit][lock] == timestamp
    requests = None  # self.requests[unit][lock] == timestamp

    def __init__(self, relation_key='coordinator', peer_relation_name=None):
        '''Instatiate a Coordinator.

        Data is stored on the peers relation and in leadership storage
        under the provided relation_key.

        The peers relation is identified by peer_relation_name, and defaults
        to the first one found in metadata.yaml.
        '''
        # Most initialization is deferred, since invoking hook tools from
        # the constructor makes testing hard.
        self.key = relation_key
        self.relname = peer_relation_name
        hookenv.atstart(self.initialize)

        # Ensure that handle() is called, without placing that burden on
        # the charm author. They still need to do this manually if they
        # are not using a hook framework.
        hookenv.atstart(self.handle)

    def initialize(self):
        if self.requests is not None:
            return  # Already initialized.

        assert hookenv.has_juju_version('1.23'), 'Needs Juju 1.23+'

        if self.relname is None:
            self.relname = _implicit_peer_relation_name()

        relids = hookenv.relation_ids(self.relname)
        if relids:
            self.relid = sorted(relids)[0]

        # Load our state, from leadership, the peer relationship, and maybe
        # local state as a fallback. Populates self.requests and self.grants.
        self._load_state()
        self._emit_state()

        # Save our state if the hook completes successfully.
        hookenv.atexit(self._save_state)

        # Schedule release of granted locks for the end of the hook.
        # This needs to be the last of our atexit callbacks to ensure
        # it will be run first when the hook is complete, because there
        # is no point mutating our state after it has been saved.
        hookenv.atexit(self._release_granted)

    def acquire(self, lock):
        '''Acquire the named lock, non-blocking.

        The lock may be granted immediately, or in a future hook.

        Returns True if the lock has been granted. The lock will be
        automatically released at the end of the hook in which it is
        granted.

        Do not mindlessly call this method, as it triggers a cascade of
        hooks. For example, if you call acquire() every time in your
        peers relation-changed hook you will end up with an infinite loop
        of hooks. It should almost always be guarded by some condition.
        '''
        unit = hookenv.local_unit()
        ts = self.requests[unit].get(lock)
        if not ts:
            # If there is no outstanding request on the peers relation,
            # create one.
            self.requests.setdefault(lock, {})
            self.requests[unit][lock] = _timestamp()
            self.msg('Requested {}'.format(lock))

        # If the leader has granted the lock, yay.
        if self.granted(lock):
            self.msg('Acquired {}'.format(lock))
            return True

        # If the unit making the request also happens to be the
        # leader, it must handle the request now. Even though the
        # request has been stored on the peers relation, the peers
        # relation-changed hook will not be triggered.
        if hookenv.is_leader():
            return self.grant(lock, unit)

        return False  # Can't acquire lock, yet. Maybe next hook.

    def granted(self, lock):
        '''Return True if a previously requested lock has been granted'''
        unit = hookenv.local_unit()
        ts = self.requests[unit].get(lock)
        if ts and self.grants.get(unit, {}).get(lock) == ts:
            return True
        return False

    def requested(self, lock):
        '''Return True if we are in the queue for the lock'''
        return lock in self.requests[hookenv.local_unit()]

    def request_timestamp(self, lock):
        '''Return the timestamp of our outstanding request for lock, or None.

        Returns a datetime.datetime() UTC timestamp, with no tzinfo attribute.
        '''
        ts = self.requests[hookenv.local_unit()].get(lock, None)
        if ts is not None:
            return datetime.strptime(ts, _timestamp_format)

    def handle(self):
        if not hookenv.is_leader():
            return  # Only the leader can grant requests.

        self.msg('Leader handling coordinator requests')

        # Clear our grants that have been released.
        for unit in self.grants.keys():
            for lock, grant_ts in list(self.grants[unit].items()):
                req_ts = self.requests.get(unit, {}).get(lock)
                if req_ts != grant_ts:
                    # The request timestamp does not match the granted
                    # timestamp. Several hooks on 'unit' may have run
                    # before the leader got a chance to make a decision,
                    # and 'unit' may have released its lock and attempted
                    # to reacquire it. This will change the timestamp,
                    # and we correctly revoke the old grant putting it
                    # to the end of the queue.
                    ts = datetime.strptime(self.grants[unit][lock],
                                           _timestamp_format)
                    del self.grants[unit][lock]
                    self.released(unit, lock, ts)

        # Grant locks
        for unit in self.requests.keys():
            for lock in self.requests[unit]:
                self.grant(lock, unit)

    def grant(self, lock, unit):
        '''Maybe grant the lock to a unit.

        The decision to grant the lock or not is made for $lock
        by a corresponding method grant_$lock, which you may define
        in a subclass. If no such method is defined, the default_grant
        method is used. See Serial.default_grant() for details.
        '''
        if not hookenv.is_leader():
            return False  # Not the leader, so we cannot grant.

        # Set of units already granted the lock.
        granted = set()
        for u in self.grants:
            if lock in self.grants[u]:
                granted.add(u)
        if unit in granted:
            return True  # Already granted.

        # Ordered list of units waiting for the lock.
        reqs = set()
        for u in self.requests:
            if u in granted:
                continue  # In the granted set. Not wanted in the req list.
            for _lock, ts in self.requests[u].items():
                if _lock == lock:
                    reqs.add((ts, u))
        queue = [t[1] for t in sorted(reqs)]
        if unit not in queue:
            return False  # Unit has not requested the lock.

        # Locate custom logic, or fallback to the default.
        grant_func = getattr(self, 'grant_{}'.format(lock), self.default_grant)

        if grant_func(lock, unit, granted, queue):
            # Grant the lock.
            self.msg('Leader grants {} to {}'.format(lock, unit))
            self.grants.setdefault(unit, {})[lock] = self.requests[unit][lock]
            return True

        return False

    def released(self, unit, lock, timestamp):
        '''Called on the leader when it has released a lock.

        By default, does nothing but log messages. Override if you
        need to perform additional housekeeping when a lock is released,
        for example recording timestamps.
        '''
        interval = _utcnow() - timestamp
        self.msg('Leader released {} from {}, held {}'.format(lock, unit,
                                                              interval))

    def require(self, lock, guard_func, *guard_args, **guard_kw):
        """Decorate a function to be run only when a lock is acquired.

        The lock is requested if the guard function returns True.

        The decorated function is called if the lock has been granted.
        """
        def decorator(f):
            @wraps(f)
            def wrapper(*args, **kw):
                if self.granted(lock):
                    self.msg('Granted {}'.format(lock))
                    return f(*args, **kw)
                if guard_func(*guard_args, **guard_kw) and self.acquire(lock):
                    return f(*args, **kw)
                return None
            return wrapper
        return decorator

    def msg(self, msg):
        '''Emit a message. Override to customize log spam.'''
        hookenv.log('coordinator.{} {}'.format(self._name(), msg),
                    level=hookenv.INFO)

    def _name(self):
        return self.__class__.__name__

    def _load_state(self):
        self.msg('Loading state'.format(self._name()))

        # All responses must be stored in the leadership settings.
        # The leader cannot use local state, as a different unit may
        # be leader next time. Which is fine, as the leadership
        # settings are always available.
        self.grants = json.loads(hookenv.leader_get(self.key) or '{}')

        local_unit = hookenv.local_unit()

        # All requests must be stored on the peers relation. This is
        # the only channel units have to communicate with the leader.
        # Even the leader needs to store its requests here, as a
        # different unit may be leader by the time the request can be
        # granted.
        if self.relid is None:
            # The peers relation is not available. Maybe we are early in
            # the units's lifecycle. Maybe this unit is standalone.
            # Fallback to using local state.
            self.msg('No peer relation. Loading local state')
            self.requests = {local_unit: self._load_local_state()}
        else:
            self.requests = self._load_peer_state()
            if local_unit not in self.requests:
                # The peers relation has just been joined. Update any state
                # loaded from our peers with our local state.
                self.msg('New peer relation. Merging local state')
                self.requests[local_unit] = self._load_local_state()

    def _emit_state(self):
        # Emit this units lock status.
        for lock in sorted(self.requests[hookenv.local_unit()].keys()):
            if self.granted(lock):
                self.msg('Granted {}'.format(lock))
            else:
                self.msg('Waiting on {}'.format(lock))

    def _save_state(self):
        self.msg('Publishing state'.format(self._name()))
        if hookenv.is_leader():
            # sort_keys to ensure stability.
            raw = json.dumps(self.grants, sort_keys=True)
            hookenv.leader_set({self.key: raw})

        local_unit = hookenv.local_unit()

        if self.relid is None:
            # No peers relation yet. Fallback to local state.
            self.msg('No peer relation. Saving local state')
            self._save_local_state(self.requests[local_unit])
        else:
            # sort_keys to ensure stability.
            raw = json.dumps(self.requests[local_unit], sort_keys=True)
            hookenv.relation_set(self.relid, relation_settings={self.key: raw})

    def _load_peer_state(self):
        requests = {}
        units = set(hookenv.related_units(self.relid))
        units.add(hookenv.local_unit())
        for unit in units:
            raw = hookenv.relation_get(self.key, unit, self.relid)
            if raw:
                requests[unit] = json.loads(raw)
        return requests

    def _local_state_filename(self):
        # Include the class name. We allow multiple BaseCoordinator
        # subclasses to be instantiated, and they are singletons, so
        # this avoids conflicts (unless someone creates and uses two
        # BaseCoordinator subclasses with the same class name, so don't
        # do that).
        return '.charmhelpers.coordinator.{}'.format(self._name())

    def _load_local_state(self):
        fn = self._local_state_filename()
        if os.path.exists(fn):
            with open(fn, 'r') as f:
                return json.load(f)
        return {}

    def _save_local_state(self, state):
        fn = self._local_state_filename()
        with open(fn, 'w') as f:
            json.dump(state, f)

    def _release_granted(self):
        # At the end of every hook, release all locks granted to
        # this unit. If a hook neglects to make use of what it
        # requested, it will just have to make the request again.
        # Implicit release is the only way this will work, as
        # if the unit is standalone there may be no future triggers
        # called to do a manual release.
        unit = hookenv.local_unit()
        for lock in list(self.requests[unit].keys()):
            if self.granted(lock):
                self.msg('Released local {} lock'.format(lock))
                del self.requests[unit][lock]


class Serial(BaseCoordinator):
    def default_grant(self, lock, unit, granted, queue):
        '''Default logic to grant a lock to a unit. Unless overridden,
        only one unit may hold the lock and it will be granted to the
        earliest queued request.

        To define custom logic for $lock, create a subclass and
        define a grant_$lock method.

        `unit` is the unit name making the request.

        `granted` is the set of units already granted the lock. It will
        never include `unit`. It may be empty.

        `queue` is the list of units waiting for the lock, ordered by time
        of request. It will always include `unit`, but `unit` is not
        necessarily first.

        Returns True if the lock should be granted to `unit`.
        '''
        return unit == queue[0] and not granted


def _implicit_peer_relation_name():
    md = hookenv.metadata()
    assert 'peers' in md, 'No peer relations in metadata.yaml'
    return sorted(md['peers'].keys())[0]


# A human readable, sortable UTC timestamp format.
_timestamp_format = '%Y-%m-%d %H:%M:%S.%fZ'


def _utcnow():  # pragma: no cover
    # This wrapper exists as mocking datetime methods is problematic.
    return datetime.utcnow()


def _timestamp():
    return _utcnow().strftime(_timestamp_format)
