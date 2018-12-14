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

import os
import json
from inspect import getargspec
from collections import Iterable, OrderedDict

from charmhelpers.core import host
from charmhelpers.core import hookenv


__all__ = ['ServiceManager', 'ManagerCallback',
           'PortManagerCallback', 'open_ports', 'close_ports', 'manage_ports',
           'service_restart', 'service_stop']


class ServiceManager(object):
    def __init__(self, services=None):
        """
        Register a list of services, given their definitions.

        Service definitions are dicts in the following formats (all keys except
        'service' are optional)::

            {
                "service": <service name>,
                "required_data": <list of required data contexts>,
                "provided_data": <list of provided data contexts>,
                "data_ready": <one or more callbacks>,
                "data_lost": <one or more callbacks>,
                "start": <one or more callbacks>,
                "stop": <one or more callbacks>,
                "ports": <list of ports to manage>,
            }

        The 'required_data' list should contain dicts of required data (or
        dependency managers that act like dicts and know how to collect the data).
        Only when all items in the 'required_data' list are populated are the list
        of 'data_ready' and 'start' callbacks executed.  See `is_ready()` for more
        information.

        The 'provided_data' list should contain relation data providers, most likely
        a subclass of :class:`charmhelpers.core.services.helpers.RelationContext`,
        that will indicate a set of data to set on a given relation.

        The 'data_ready' value should be either a single callback, or a list of
        callbacks, to be called when all items in 'required_data' pass `is_ready()`.
        Each callback will be called with the service name as the only parameter.
        After all of the 'data_ready' callbacks are called, the 'start' callbacks
        are fired.

        The 'data_lost' value should be either a single callback, or a list of
        callbacks, to be called when a 'required_data' item no longer passes
        `is_ready()`.  Each callback will be called with the service name as the
        only parameter.  After all of the 'data_lost' callbacks are called,
        the 'stop' callbacks are fired.

        The 'start' value should be either a single callback, or a list of
        callbacks, to be called when starting the service, after the 'data_ready'
        callbacks are complete.  Each callback will be called with the service
        name as the only parameter.  This defaults to
        `[host.service_start, services.open_ports]`.

        The 'stop' value should be either a single callback, or a list of
        callbacks, to be called when stopping the service.  If the service is
        being stopped because it no longer has all of its 'required_data', this
        will be called after all of the 'data_lost' callbacks are complete.
        Each callback will be called with the service name as the only parameter.
        This defaults to `[services.close_ports, host.service_stop]`.

        The 'ports' value should be a list of ports to manage.  The default
        'start' handler will open the ports after the service is started,
        and the default 'stop' handler will close the ports prior to stopping
        the service.


        Examples:

        The following registers an Upstart service called bingod that depends on
        a mongodb relation and which runs a custom `db_migrate` function prior to
        restarting the service, and a Runit service called spadesd::

            manager = services.ServiceManager([
                {
                    'service': 'bingod',
                    'ports': [80, 443],
                    'required_data': [MongoRelation(), config(), {'my': 'data'}],
                    'data_ready': [
                        services.template(source='bingod.conf'),
                        services.template(source='bingod.ini',
                                          target='/etc/bingod.ini',
                                          owner='bingo', perms=0400),
                    ],
                },
                {
                    'service': 'spadesd',
                    'data_ready': services.template(source='spadesd_run.j2',
                                                    target='/etc/sv/spadesd/run',
                                                    perms=0555),
                    'start': runit_start,
                    'stop': runit_stop,
                },
            ])
            manager.manage()
        """
        self._ready_file = os.path.join(hookenv.charm_dir(), 'READY-SERVICES.json')
        self._ready = None
        self.services = OrderedDict()
        for service in services or []:
            service_name = service['service']
            self.services[service_name] = service

    def manage(self):
        """
        Handle the current hook by doing The Right Thing with the registered services.
        """
        hookenv._run_atstart()
        try:
            hook_name = hookenv.hook_name()
            if hook_name == 'stop':
                self.stop_services()
            else:
                self.reconfigure_services()
                self.provide_data()
        except SystemExit as x:
            if x.code is None or x.code == 0:
                hookenv._run_atexit()
        hookenv._run_atexit()

    def provide_data(self):
        """
        Set the relation data for each provider in the ``provided_data`` list.

        A provider must have a `name` attribute, which indicates which relation
        to set data on, and a `provide_data()` method, which returns a dict of
        data to set.

        The `provide_data()` method can optionally accept two parameters:

          * ``remote_service`` The name of the remote service that the data will
            be provided to.  The `provide_data()` method will be called once
            for each connected service (not unit).  This allows the method to
            tailor its data to the given service.
          * ``service_ready`` Whether or not the service definition had all of
            its requirements met, and thus the ``data_ready`` callbacks run.

        Note that the ``provided_data`` methods are now called **after** the
        ``data_ready`` callbacks are run.  This gives the ``data_ready`` callbacks
        a chance to generate any data necessary for the providing to the remote
        services.
        """
        for service_name, service in self.services.items():
            service_ready = self.is_ready(service_name)
            for provider in service.get('provided_data', []):
                for relid in hookenv.relation_ids(provider.name):
                    units = hookenv.related_units(relid)
                    if not units:
                        continue
                    remote_service = units[0].split('/')[0]
                    argspec = getargspec(provider.provide_data)
                    if len(argspec.args) > 1:
                        data = provider.provide_data(remote_service, service_ready)
                    else:
                        data = provider.provide_data()
                    if data:
                        hookenv.relation_set(relid, data)

    def reconfigure_services(self, *service_names):
        """
        Update all files for one or more registered services, and,
        if ready, optionally restart them.

        If no service names are given, reconfigures all registered services.
        """
        for service_name in service_names or self.services.keys():
            if self.is_ready(service_name):
                self.fire_event('data_ready', service_name)
                self.fire_event('start', service_name, default=[
                    service_restart,
                    manage_ports])
                self.save_ready(service_name)
            else:
                if self.was_ready(service_name):
                    self.fire_event('data_lost', service_name)
                self.fire_event('stop', service_name, default=[
                    manage_ports,
                    service_stop])
                self.save_lost(service_name)

    def stop_services(self, *service_names):
        """
        Stop one or more registered services, by name.

        If no service names are given, stops all registered services.
        """
        for service_name in service_names or self.services.keys():
            self.fire_event('stop', service_name, default=[
                manage_ports,
                service_stop])

    def get_service(self, service_name):
        """
        Given the name of a registered service, return its service definition.
        """
        service = self.services.get(service_name)
        if not service:
            raise KeyError('Service not registered: %s' % service_name)
        return service

    def fire_event(self, event_name, service_name, default=None):
        """
        Fire a data_ready, data_lost, start, or stop event on a given service.
        """
        service = self.get_service(service_name)
        callbacks = service.get(event_name, default)
        if not callbacks:
            return
        if not isinstance(callbacks, Iterable):
            callbacks = [callbacks]
        for callback in callbacks:
            if isinstance(callback, ManagerCallback):
                callback(self, service_name, event_name)
            else:
                callback(service_name)

    def is_ready(self, service_name):
        """
        Determine if a registered service is ready, by checking its 'required_data'.

        A 'required_data' item can be any mapping type, and is considered ready
        if `bool(item)` evaluates as True.
        """
        service = self.get_service(service_name)
        reqs = service.get('required_data', [])
        return all(bool(req) for req in reqs)

    def _load_ready_file(self):
        if self._ready is not None:
            return
        if os.path.exists(self._ready_file):
            with open(self._ready_file) as fp:
                self._ready = set(json.load(fp))
        else:
            self._ready = set()

    def _save_ready_file(self):
        if self._ready is None:
            return
        with open(self._ready_file, 'w') as fp:
            json.dump(list(self._ready), fp)

    def save_ready(self, service_name):
        """
        Save an indicator that the given service is now data_ready.
        """
        self._load_ready_file()
        self._ready.add(service_name)
        self._save_ready_file()

    def save_lost(self, service_name):
        """
        Save an indicator that the given service is no longer data_ready.
        """
        self._load_ready_file()
        self._ready.discard(service_name)
        self._save_ready_file()

    def was_ready(self, service_name):
        """
        Determine if the given service was previously data_ready.
        """
        self._load_ready_file()
        return service_name in self._ready


class ManagerCallback(object):
    """
    Special case of a callback that takes the `ServiceManager` instance
    in addition to the service name.

    Subclasses should implement `__call__` which should accept three parameters:

        * `manager`       The `ServiceManager` instance
        * `service_name`  The name of the service it's being triggered for
        * `event_name`    The name of the event that this callback is handling
    """
    def __call__(self, manager, service_name, event_name):
        raise NotImplementedError()


class PortManagerCallback(ManagerCallback):
    """
    Callback class that will open or close ports, for use as either
    a start or stop action.
    """
    def __call__(self, manager, service_name, event_name):
        service = manager.get_service(service_name)
        # turn this generator into a list,
        # as we'll be going over it multiple times
        new_ports = list(service.get('ports', []))
        port_file = os.path.join(hookenv.charm_dir(), '.{}.ports'.format(service_name))
        if os.path.exists(port_file):
            with open(port_file) as fp:
                old_ports = fp.read().split(',')
            for old_port in old_ports:
                if bool(old_port) and not self.ports_contains(old_port, new_ports):
                    hookenv.close_port(old_port)
        with open(port_file, 'w') as fp:
            fp.write(','.join(str(port) for port in new_ports))
        for port in new_ports:
            # A port is either a number or 'ICMP'
            protocol = 'TCP'
            if str(port).upper() == 'ICMP':
                protocol = 'ICMP'
            if event_name == 'start':
                hookenv.open_port(port, protocol)
            elif event_name == 'stop':
                hookenv.close_port(port, protocol)

    def ports_contains(self, port, ports):
        if not bool(port):
            return False
        if str(port).upper() != 'ICMP':
            port = int(port)
        return port in ports


def service_stop(service_name):
    """
    Wrapper around host.service_stop to prevent spurious "unknown service"
    messages in the logs.
    """
    if host.service_running(service_name):
        host.service_stop(service_name)


def service_restart(service_name):
    """
    Wrapper around host.service_restart to prevent spurious "unknown service"
    messages in the logs.
    """
    if host.service_available(service_name):
        if host.service_running(service_name):
            host.service_restart(service_name)
        else:
            host.service_start(service_name)


# Convenience aliases
open_ports = close_ports = manage_ports = PortManagerCallback()
