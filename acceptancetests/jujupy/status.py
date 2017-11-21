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

import json
import re
import yaml

from collections import defaultdict
from datetime import datetime
from dateutil.parser import parse as datetime_parse
from dateutil import tz

from jujupy.exceptions import (
    AgentError,
    AgentUnresolvedError,
    AppError,
    ErroredUnit,
    HookFailedError,
    InstallError,
    MachineError,
    ProvisioningError,
    StuckAllocatingError,
    UnitError,
)
from jujupy.utility import (
    _dns_name_for_machine,
    )

__metaclass__ = type


AGENTS_READY = set(['started', 'idle'])


def coalesce_agent_status(agent_item):
    """Return the machine agent-state or the unit agent-status."""
    state = agent_item.get('agent-state')
    if state is None and agent_item.get('agent-status') is not None:
        state = agent_item.get('agent-status').get('current')
    if state is None and agent_item.get('juju-status') is not None:
        state = agent_item.get('juju-status').get('current')
    if state is None:
        state = 'no-agent'
    return state


class StatusItem:

    APPLICATION = 'application-status'
    WORKLOAD = 'workload-status'
    MACHINE = 'machine-status'
    JUJU = 'juju-status'

    def __init__(self, status_name, item_name, item_value):
        """Create a new StatusItem from its fields.

        :param status_name: One of the status strings.
        :param item_name: The name of the machine/unit/application the status
            information is about.
        :param item_value: A dictionary of status values. If there is an entry
            with the status_name in the dictionary its contents are used."""
        self.status_name = status_name
        self.item_name = item_name
        self.status = item_value.get(status_name, item_value)

    def __eq__(self, other):
        if type(other) != type(self):
            return False
        elif self.status_name != other.status_name:
            return False
        elif self.item_name != other.item_name:
            return False
        elif self.status != other.status:
            return False
        else:
            return True

    def __ne__(self, other):
        return bool(not self == other)

    @property
    def message(self):
        return self.status.get('message')

    @property
    def since(self):
        return self.status.get('since')

    @property
    def current(self):
        return self.status.get('current')

    @property
    def version(self):
        return self.status.get('version')

    @property
    def datetime_since(self):
        if self.since is None:
            return None
        return datetime_parse(self.since)

    def to_exception(self):
        """Create an exception representing the error if one exists.

        :return: StatusError (or subtype) to represent an error or None
        to show that there is no error."""
        if self.current not in ['error', 'failed', 'down',
                                'provisioning error']:
            if (self.current, self.status_name) != (
                    'allocating', self.MACHINE):
                return None
        if self.APPLICATION == self.status_name:
            return AppError(self.item_name, self.message)
        elif self.WORKLOAD == self.status_name:
            if self.message is None:
                return UnitError(self.item_name, self.message)
            elif re.match('hook failed: ".*install.*"', self.message):
                return InstallError(self.item_name, self.message)
            elif re.match('hook failed', self.message):
                return HookFailedError(self.item_name, self.message)
            else:
                return UnitError(self.item_name, self.message)
        elif self.MACHINE == self.status_name:
            if self.current == 'provisioning error':
                return ProvisioningError(self.item_name, self.message)
            if self.current == 'allocating':
                return StuckAllocatingError(
                    self.item_name,
                    'Stuck allocating.  Last message: {}'.format(self.message))
            else:
                return MachineError(self.item_name, self.message)
        elif self.JUJU == self.status_name:
            if self.since is None:
                return AgentError(self.item_name, self.message)
            time_since = datetime.now(tz.gettz('UTC')) - self.datetime_since
            if time_since > AgentUnresolvedError.a_reasonable_time:
                return AgentUnresolvedError(self.item_name, self.message,
                                            time_since.total_seconds())
            else:
                return AgentError(self.item_name, self.message)
        else:
            raise ValueError('Unknown status:{}'.format(self.status_name),
                             (self.item_name, self.status_value))

    def __repr__(self):
        return 'StatusItem({!r}, {!r}, {!r})'.format(
            self.status_name, self.item_name, self.status)


class Status:

    def __init__(self, status, status_text):
        self.status = status
        self.status_text = status_text

    @classmethod
    def from_text(cls, text):
        try:
            # Parsing as JSON is much faster than parsing as YAML, so try
            # parsing as JSON first and fall back to YAML.
            status_yaml = json.loads(text)
        except ValueError:
            status_yaml = yaml.safe_load(text)
        return cls(status_yaml, text)

    @property
    def model_name(self):
        return self.status['model']['name']

    def get_applications(self):
        return self.status.get('applications', {})

    def iter_machines(self, containers=False, machines=True):
        for machine_name, machine in sorted(self.status['machines'].items()):
            if machines:
                yield machine_name, machine
            if containers:
                for contained, unit in machine.get('containers', {}).items():
                    yield contained, unit

    def iter_new_machines(self, old_status, containers=False):
        old = dict(old_status.iter_machines(containers=containers))
        for machine, data in self.iter_machines(containers=containers):
            if machine in old:
                continue
            yield machine, data

    def _iter_units_in_application(self, app_data):
        """Given application data, iterate through every unit in it."""
        for unit_name, unit in sorted(app_data.get('units', {}).items()):
            yield unit_name, unit
            subordinates = unit.get('subordinates', ())
            for sub_name in sorted(subordinates):
                yield sub_name, subordinates[sub_name]

    def iter_units(self):
        """Iterate over every unit in every application."""
        for service_name, service in sorted(self.get_applications().items()):
            for name, data in self._iter_units_in_application(service):
                yield name, data

    def agent_items(self):
        for machine_name, machine in self.iter_machines(containers=True):
            yield machine_name, machine
        for unit_name, unit in self.iter_units():
            yield unit_name, unit

    def unit_agent_states(self, states=None):
        """Fill in a dictionary with the states of units.

        Units of a dying application are marked as dying.

        :param states: If not None, when it should be a defaultdict(list)),
        then states are added to this dictionary."""
        if states is None:
            states = defaultdict(list)
        for app_name, app_data in sorted(self.get_applications().items()):
            if app_data.get('life') == 'dying':
                for unit, data in self._iter_units_in_application(app_data):
                    states['dying'].append(unit)
            else:
                for unit, data in self._iter_units_in_application(app_data):
                    states[coalesce_agent_status(data)].append(unit)
        return states

    def agent_states(self):
        """Map agent states to the units and machines in those states."""
        states = defaultdict(list)
        for item_name, item in self.iter_machines(containers=True):
            states[coalesce_agent_status(item)].append(item_name)
        self.unit_agent_states(states)
        return states

    def check_agents_started(self, environment_name=None):
        """Check whether all agents are in the 'started' state.

        If not, return agent_states output.  If so, return None.
        If an error is encountered for an agent, raise ErroredUnit
        """
        bad_state_info = re.compile(
            '(.*error|^(cannot set up groups|cannot run instance)).*')
        for item_name, item in self.agent_items():
            state_info = item.get('agent-state-info', '')
            if bad_state_info.match(state_info):
                raise ErroredUnit(item_name, state_info)
        states = self.agent_states()
        if set(states.keys()).issubset(AGENTS_READY):
            return None
        for state, entries in states.items():
            if 'error' in state:
                # sometimes the state may be hidden in juju status message
                juju_status = dict(
                    self.agent_items())[entries[0]].get('juju-status')
                if juju_status:
                    juju_status_msg = juju_status.get('message')
                    if juju_status_msg:
                        state = juju_status_msg
                raise ErroredUnit(entries[0], state)
        return states

    def get_service_count(self):
        return len(self.get_applications())

    def get_service_unit_count(self, service):
        return len(
            self.get_applications().get(service, {}).get('units', {}))

    def get_agent_versions(self):
        versions = defaultdict(set)
        for item_name, item in self.agent_items():
            if item.get('juju-status', None):
                version = item['juju-status'].get('version', 'unknown')
                versions[version].add(item_name)
            else:
                versions[item.get('agent-version', 'unknown')].add(item_name)
        return versions

    def get_instance_id(self, machine_id):
        return self.status['machines'][machine_id]['instance-id']

    def get_machine_dns_name(self, machine_id):
        return _dns_name_for_machine(self, machine_id)

    def get_unit(self, unit_name):
        """Return metadata about a unit."""
        for name, service in sorted(self.get_applications().items()):
            units = service.get('units', {})
            if unit_name in units:
                return service['units'][unit_name]
            # The unit might be a subordinate, in which case it won't
            # be under its application, but under the principal
            # unit.
            for _, unit in units.items():
                if unit_name in unit.get('subordinates', {}):
                    return unit['subordinates'][unit_name]
        raise KeyError(unit_name)

    def service_subordinate_units(self, service_name):
        """Return subordinate metadata for a service_name."""
        services = self.get_applications()
        if service_name in services:
            for name, unit in sorted(services[service_name].get(
                    'units', {}).items()):
                for sub_name, sub in unit.get('subordinates', {}).items():
                    yield sub_name, sub

    def get_open_ports(self, unit_name):
        """List the open ports for the specified unit.

        If no ports are listed for the unit, the empty list is returned.
        """
        return self.get_unit(unit_name).get('open-ports', [])

    def iter_status(self):
        """Iterate through every status field in the larger status data."""
        for machine_name, machine_value in self.iter_machines(containers=True):
            yield StatusItem(StatusItem.MACHINE, machine_name, machine_value)
            yield StatusItem(StatusItem.JUJU, machine_name, machine_value)
        for app_name, app_value in self.get_applications().items():
            yield StatusItem(StatusItem.APPLICATION, app_name, app_value)
            unit_iterator = self._iter_units_in_application(app_value)
            for unit_name, unit_value in unit_iterator:
                yield StatusItem(StatusItem.WORKLOAD, unit_name, unit_value)
                yield StatusItem(StatusItem.JUJU, unit_name, unit_value)

    def iter_errors(self, ignore_recoverable=False):
        """Iterate through every error, repersented by exceptions."""
        for sub_status in self.iter_status():
            error = sub_status.to_exception()
            if error is not None:
                if not (ignore_recoverable and error.recoverable):
                    yield error

    def check_for_errors(self, ignore_recoverable=False):
        """Return a list of errors, in order of their priority."""
        return sorted(self.iter_errors(ignore_recoverable),
                      key=lambda item: item.priority())

    def raise_highest_error(self, ignore_recoverable=False):
        """Raise an exception reperenting the highest priority error."""
        errors = self.check_for_errors(ignore_recoverable)
        if errors:
            raise errors[0]
