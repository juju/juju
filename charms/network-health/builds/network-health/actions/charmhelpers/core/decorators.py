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

#
# Copyright 2014 Canonical Ltd.
#
# Authors:
#  Edward Hope-Morley <opentastic@gmail.com>
#

import time

from charmhelpers.core.hookenv import (
    log,
    INFO,
)


def retry_on_exception(num_retries, base_delay=0, exc_type=Exception):
    """If the decorated function raises exception exc_type, allow num_retries
    retry attempts before raise the exception.
    """
    def _retry_on_exception_inner_1(f):
        def _retry_on_exception_inner_2(*args, **kwargs):
            retries = num_retries
            multiplier = 1
            while True:
                try:
                    return f(*args, **kwargs)
                except exc_type:
                    if not retries:
                        raise

                delay = base_delay * multiplier
                multiplier += 1
                log("Retrying '%s' %d more times (delay=%s)" %
                    (f.__name__, retries, delay), level=INFO)
                retries -= 1
                if delay:
                    time.sleep(delay)

        return _retry_on_exception_inner_2

    return _retry_on_exception_inner_1
