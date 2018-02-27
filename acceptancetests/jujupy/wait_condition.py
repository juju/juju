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
from datetime import datetime
from time import sleep
from subprocess import CalledProcessError

from jujupy.exceptions import (
    VersionsNotUpdated,
    AgentsNotStarted,
    )
from jujupy.status import (
    Status,
    )
from utility import (
    until_timeout,
)


log = logging.getLogger(__name__)

__metaclass__ = type


class ModelCheckFailed(Exception):
    """Exception used to signify a model status check failed or timed out."""


def wait_for_model_check(client, model_check, timeout):
    """Wrapper to have a client wait for a model_check callable to succeed.

    :param client: ModelClient object to act on and pass into model_check
    :param model_check: Callable that takes a ModelClient object. When the
      callable reaches a success state it returns True. If model_check never
      returns True within `timeout`, the exception ModelCheckFailed will be
      raised.
    """
    with client.check_timeouts():
        with client.ignore_soft_deadline():
            for _ in until_timeout(timeout):
                if model_check(client):
                    return
                sleep(1)
    raise ModelCheckFailed()


def wait_until_model_upgrades(client, timeout=300):
    # Poll using a command that will fail until the upgrade is complete.
    def model_upgrade_status_check(client):
        try:
            log.info('Attempting API connection, failure is not fatal.')
            client.juju('list-users', (), include_e=False)
            return True
        except CalledProcessError:
            # Upgrade will still be in progress and thus refuse the api call.
            return False
    try:
        wait_for_model_check(client, model_upgrade_status_check, timeout)
    except ModelCheckFailed:
        raise AssertionError(
            'Upgrade for model {} failed to complete within the alloted '
            'timeout ({} seconds)'.format(
                client.model_name, timeout))


class BaseCondition:
    """Base class for conditions that support client.wait_for."""

    def __init__(self, timeout=300, already_satisfied=False):
        self.timeout = timeout
        self.already_satisfied = already_satisfied

    def iter_blocking_state(self, status):
        """Identify when the condition required is met.

        When the operation is complete yield nothing. Otherwise yields a
        tuple ('<item detail>', '<state>')
        as to why the action cannot be considered complete yet.

        An example for a condition of an application being removed:
            yield <application name>, 'still-present'
        """
        raise NotImplementedError()

    def do_raise(self, model_name, status):
        """Raise exception for when success condition fails to be achieved."""
        raise NotImplementedError()


class ConditionList(BaseCondition):
    """A list of conditions that support client.wait_for.

    This combines the supplied list of conditions.  It is only satisfied when
    all conditions are met.  It times out when any member times out.  When
    asked to raise, it causes the first condition to raise an exception.  An
    improvement would be to raise the first condition whose timeout has been
    exceeded.
    """

    def __init__(self, conditions):
        if len(conditions) == 0:
            timeout = 300
        else:
            timeout = max(c.timeout for c in conditions)
        already_satisfied = all(c.already_satisfied for c in conditions)
        super(ConditionList, self).__init__(timeout, already_satisfied)
        self._conditions = conditions

    def iter_blocking_state(self, status):
        for condition in self._conditions:
            for item, state in condition.iter_blocking_state(status):
                yield item, state

    def do_raise(self, model_name, status):
        self._conditions[0].do_raise(model_name, status)


class NoopCondition(BaseCondition):

    def iter_blocking_state(self, status):
        return iter(())

    def do_raise(self, model_name, status):
        raise Exception('NoopCondition failed: {}'.format(model_name))


class AllApplicationActive(BaseCondition):
    """Ensure all applications (incl. subordinates) are 'active' state."""

    def iter_blocking_state(self, status):
        applications = status.get_applications()
        all_app_status = [
            state['application-status']['current']
            for name, state in applications.items()]
        apps_active = [state == 'active' for state in all_app_status]
        if not all(apps_active):
            yield 'applications', 'not-all-active'

    def do_raise(self, model_name, status):
        raise Exception('Timed out waiting for all applications to be active.')


class AllApplicationWorkloads(BaseCondition):
    """Ensure all applications (incl. subordinates) are workload 'active'."""

    def iter_blocking_state(self, status):
        app_workloads_active = []
        for name, unit in status.iter_units():
            try:
                state = unit['workload-status']['current'] == 'active'
            except KeyError:
                state = False
            app_workloads_active.append(state)
        if not all(app_workloads_active):
            yield 'application-workloads', 'not-all-active'

    def do_raise(self, model_name, status):
        raise Exception(
            'Timed out waiting for all application workloads to be active.')


class AgentsIdle(BaseCondition):
    """Ensure all specified agents are finished doing setup work."""

    def __init__(self, units, *args, **kws):
        self.units = units
        super(AgentsIdle, self).__init__(*args, **kws)

    def iter_blocking_state(self, status):
        idles = []
        for name in self.units:
            try:
                unit = status.get_unit(name)
                state = unit['juju-status']['current'] == 'idle'
            except KeyError:
                state = False
            idles.append(state)
        if not all(idles):
            yield 'application-agents', 'not-all-idle'

    def do_raise(self, model_name, status):
        raise Exception("Timed out waiting for all agents to be idle.")


class WaitMachineNotPresent(BaseCondition):
    """Condition satisfied when a given machine is not present."""

    def __init__(self, machine, timeout=300):
        super(WaitMachineNotPresent, self).__init__(timeout)
        self.machine = machine

    def __eq__(self, other):
        if not type(self) is type(other):
            return False
        if self.timeout != other.timeout:
            return False
        if self.machine != other.machine:
            return False
        return True

    def __ne__(self, other):
        return not self.__eq__(other)

    def iter_blocking_state(self, status):
        for machine, info in status.iter_machines():
            if machine == self.machine:
                yield machine, 'still-present'

    def do_raise(self, model_name, status):
        raise Exception("Timed out waiting for machine removal %s" %
                        self.machine)


class WaitApplicationNotPresent(BaseCondition):
    """Condition satisfied when a given machine is not present."""

    def __init__(self, application, timeout=300):
        super(WaitApplicationNotPresent, self).__init__(timeout)
        self.application = application

    def __eq__(self, other):
        if not type(self) is type(other):
            return False
        if self.timeout != other.timeout:
            return False
        if self.application != other.application:
            return False
        return True

    def __ne__(self, other):
        return not self.__eq__(other)

    def iter_blocking_state(self, status):
        for application in status.get_applications().keys():
            if application == self.application:
                yield application, 'still-present'

    def do_raise(self, model_name, status):
        raise Exception("Timed out waiting for application "
                        "removal {}".format(self.application))


class MachineDown(BaseCondition):
    """Condition satisfied when a given machine is down."""

    def __init__(self, machine_id):
        super(MachineDown, self).__init__()
        self.machine_id = machine_id

    def iter_blocking_state(self, status):
        """Yield the juju-status of the machine if it is not 'down'."""
        juju_status = status.status['machines'][self.machine_id]['juju-status']
        if juju_status['current'] != 'down':
            yield self.machine_id, juju_status['current']

    def do_raise(self, model_name, status):
        raise Exception(
            "Timed out waiting for juju to determine machine {} down.".format(
                self.machine_id))


class WaitVersion(BaseCondition):

    def __init__(self, target_version, timeout=300):
        super(WaitVersion, self).__init__(timeout)
        self.target_version = target_version

    def iter_blocking_state(self, status):
        for version, agents in status.get_agent_versions().items():
            if version == self.target_version:
                continue
            for agent in agents:
                yield agent, version

    def do_raise(self, model_name, status):
        raise VersionsNotUpdated(model_name, status)


class WaitModelVersion(BaseCondition):

    def __init__(self, target_version, timeout=300):
        super(WaitModelVersion, self).__init__(timeout)
        self.target_version = target_version

    def iter_blocking_state(self, status):
        model_version = status.status['model']['version']
        if model_version != self.target_version:
            yield status.model_name, model_version

    def do_raise(self, model_name, status):
        raise VersionsNotUpdated(model_name, status)


class WaitAgentsStarted(BaseCondition):
    """Wait until all agents are idle or started."""

    def __init__(self, timeout=1200):
        super(WaitAgentsStarted, self).__init__(timeout)

    def iter_blocking_state(self, status):
        states = Status.check_agents_started(status)

        if states is not None:
            for state, item in states.items():
                yield item[0], state

    def do_raise(self, model_name, status):
        raise AgentsNotStarted(model_name, status)


class CommandComplete(BaseCondition):
    """Wraps a CommandTime and gives the ability to wait_for completion."""

    def __init__(self, real_condition, command_time):
        """Constructor.

        :param real_condition: BaseCondition object.
        :param command_time: CommandTime object representing the command to
          wait for completion.
        """
        super(CommandComplete, self).__init__(
            real_condition.timeout,
            real_condition.already_satisfied)
        self._real_condition = real_condition
        self.command_time = command_time
        if real_condition.already_satisfied:
            self.command_time.actual_completion()

    def iter_blocking_state(self, status):
        """Wraps the iter_blocking_state of the stored BaseCondition.

        When the operation is complete iter_blocking_state yields nothing.
        Otherwise iter_blocking_state yields details as to why the action
        cannot be considered complete yet.
        """
        completed = True
        for item, state in self._real_condition.iter_blocking_state(status):
            completed = False
            yield item, state
        if completed:
            self.command_time.actual_completion()

    def do_raise(self, status):
        raise RuntimeError(
            'Timed out waiting for "{}" command to complete: "{}"'.format(
                self.command_time.cmd,
                ' '.join(self.command_time.full_args)))


class CommandTime:
    """Store timing details for a juju command."""

    def __init__(self, cmd, full_args, envvars=None, start=None):
        """Constructor.

        :param cmd: Command string for command run (e.g. bootstrap)
        :param args: List of all args the command was called with.
        :param envvars: Dict of any extra envvars set before command was
          called.
        :param start: datetime.datetime object representing when the command
          was run. If None defaults to datetime.utcnow()
        """
        self.cmd = cmd
        self.full_args = full_args
        self.envvars = envvars
        self.start = start if start else datetime.utcnow()
        self.end = None

    def actual_completion(self, end=None):
        """Signify that actual completion time of the command.

        Note. ignores multiple calls after the initial call.

        :param end: datetime.datetime object. If None defaults to
          datetime.datetime.utcnow()
        """
        if self.end is None:
            self.end = end if end else datetime.utcnow()

    @property
    def total_seconds(self):
        """Total amount of seconds a command took to complete.

        :return: Int representing number of seconds or None if the command
          timing has never been completed.
        """
        if self.end is None:
            return None
        return (self.end - self.start).total_seconds()
