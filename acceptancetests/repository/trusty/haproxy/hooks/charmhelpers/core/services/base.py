# Copyright 2014-2015 Canonical Limited.
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

import os
import re
import json
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
        hook_name = hookenv.hook_name()
        if hook_name == 'stop':
            self.stop_services()
        else:
            self.provide_data()
            self.reconfigure_services()
        cfg = hookenv.config()
        if cfg.implicit_save:
            cfg.save()

    def provide_data(self):
        """
        Set the relation data for each provider in the ``provided_data`` list.

        A provider must have a `name` attribute, which indicates which relation
        to set data on, and a `provide_data()` method, which returns a dict of
        data to set.
        """
        hook_name = hookenv.hook_name()
        for service in self.services.values():
            for provider in service.get('provided_data', []):
                if re.match(r'{}-relation-(joined|changed)'.format(provider.name), hook_name):
                    data = provider.provide_data()
                    _ready = provider._is_ready(data) if hasattr(provider, '_is_ready') else data
                    if _ready:
                        hookenv.relation_set(None, data)

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
        new_ports = service.get('ports', [])
        port_file = os.path.join(hookenv.charm_dir(), '.{}.ports'.format(service_name))
        if os.path.exists(port_file):
            with open(port_file) as fp:
                old_ports = fp.read().split(',')
            for old_port in old_ports:
                if bool(old_port):
                    old_port = int(old_port)
                    if old_port not in new_ports:
                        hookenv.close_port(old_port)
        with open(port_file, 'w') as fp:
            fp.write(','.join(str(port) for port in new_ports))
        for port in new_ports:
            if event_name == 'start':
                hookenv.open_port(port)
            elif event_name == 'stop':
                hookenv.close_port(port)


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
