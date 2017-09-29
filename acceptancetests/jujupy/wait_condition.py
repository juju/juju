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

from datetime import datetime

from jujupy.exceptions import (
    VersionsNotUpdated,
    AgentsNotStarted,
    )
from jujupy.status import (
    Status,
    )


__metaclass__ = type


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
