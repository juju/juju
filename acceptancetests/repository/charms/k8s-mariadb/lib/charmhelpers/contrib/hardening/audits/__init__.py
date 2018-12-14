# Copyright 2016 Canonical Limited.
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


class BaseAudit(object):  # NO-QA
    """Base class for hardening checks.

    The lifecycle of a hardening check is to first check to see if the system
    is in compliance for the specified check. If it is not in compliance, the
    check method will return a value which will be supplied to the.
    """
    def __init__(self, *args, **kwargs):
        self.unless = kwargs.get('unless', None)
        super(BaseAudit, self).__init__()

    def ensure_compliance(self):
        """Checks to see if the current hardening check is in compliance or
        not.

        If the check that is performed is not in compliance, then an exception
        should be raised.
        """
        pass

    def _take_action(self):
        """Determines whether to perform the action or not.

        Checks whether or not an action should be taken. This is determined by
        the truthy value for the unless parameter. If unless is a callback
        method, it will be invoked with no parameters in order to determine
        whether or not the action should be taken. Otherwise, the truthy value
        of the unless attribute will determine if the action should be
        performed.
        """
        # Do the action if there isn't an unless override.
        if self.unless is None:
            return True

        # Invoke the callback if there is one.
        if hasattr(self.unless, '__call__'):
            return not self.unless()

        return not self.unless
