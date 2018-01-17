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

import re
import subprocess

from datetime import timedelta


__metaclass__ = type


class StatusTimeout(Exception):
    """Raised when 'juju status' timed out."""


class SoftDeadlineExceeded(Exception):
    """Raised when an overall client operation takes too long."""

    def __init__(self):
        super(SoftDeadlineExceeded, self).__init__(
            'Operation exceeded deadline.')


class NoProvider(Exception):
    """Raised when an environment defines no provider."""


class TypeNotAccepted(Exception):
    """Raised when the provided type was not accepted."""


class NameNotAccepted(Exception):
    """Raised when the provided name was not accepted."""


class InvalidEndpoint(Exception):
    """Raised when the provided endpoint was deemed invalid."""


class AuthNotAccepted(Exception):
    """Raised when the provided auth was not accepted."""


class NoActiveModel(Exception):
    """Raised when no active model could be found."""


class NoActiveControllers(Exception):
    """Raised when no active environment could be found."""


class ErroredUnit(Exception):

    def __init__(self, unit_name, state):
        msg = '%s is in state %s' % (unit_name, state)
        Exception.__init__(self, msg)
        self.unit_name = unit_name
        self.state = state


class UpgradeMongoNotSupported(Exception):

    def __init__(self):
        super(UpgradeMongoNotSupported, self).__init__(
            'This client does not support upgrade-mongo')


class CannotConnectEnv(subprocess.CalledProcessError):

    def __init__(self, e):
        super(CannotConnectEnv, self).__init__(e.returncode, e.cmd, e.output)


class StatusNotMet(Exception):

    _fmt = 'Expected status not reached in {env}.'

    def __init__(self, environment_name, status):
        self.env = environment_name
        self.status = status

    def __str__(self):
        return self._fmt.format(env=self.env)


class AgentsNotStarted(StatusNotMet):

    _fmt = 'Timed out waiting for agents to start in {env}.'


class VersionsNotUpdated(StatusNotMet):

    _fmt = 'Some versions did not update.'


class WorkloadsNotReady(StatusNotMet):

    _fmt = 'Workloads not ready in {env}.'


class ApplicationsNotStarted(StatusNotMet):

    _fmt = 'Timed out waiting for applications to start in {env}.'


class VotingNotEnabled(StatusNotMet):

    _fmt = 'Timed out waiting for voting to be enabled in {env}.'


class StatusError(Exception):
    """Generic error for Status."""

    recoverable = True

    # This has to be filled in after the classes are declared.
    ordering = []

    @classmethod
    def priority(cls):
        """Get the priority of the StatusError as an number.

        Lower number means higher priority. This can be used as a key
        function in sorting."""
        return cls.ordering.index(cls)


class MachineError(StatusError):
    """Error in machine-status."""

    recoverable = False


class ProvisioningError(MachineError):
    """Machine experianced a 'provisioning error'."""


class StuckAllocatingError(MachineError):
    """Machine did not transition out of 'allocating' state."""

    recoverable = True


class UnitError(StatusError):
    """Error in a unit's status."""


class HookFailedError(UnitError):
    """A unit hook has failed."""

    def __init__(self, item_name, msg):
        match = re.search('^hook failed: "([^"]+)"$', msg)
        if match:
            msg = match.group(1)
        super(HookFailedError, self).__init__(item_name, msg)


class InstallError(HookFailedError):
    """The unit's install hook has failed."""

    recoverable = True


class AppError(StatusError):
    """Error in an application's status."""


class AgentError(StatusError):
    """Error in a juju agent."""


class AgentUnresolvedError(AgentError):
    """Agent error has not recovered in a reasonable time."""

    # This is the time limit set by IS for recovery from an agent error.
    a_reasonable_time = timedelta(minutes=5)


StatusError.ordering = [
    ProvisioningError, StuckAllocatingError, MachineError, InstallError,
    AgentUnresolvedError, HookFailedError, UnitError, AppError, AgentError,
    StatusError,
    ]
