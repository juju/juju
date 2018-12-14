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

from . import cmdline
from charmhelpers.core import unitdata


@cmdline.subcommand_builder('unitdata', description="Store and retrieve data")
def unitdata_cmd(subparser):
    nested = subparser.add_subparsers()
    get_cmd = nested.add_parser('get', help='Retrieve data')
    get_cmd.add_argument('key', help='Key to retrieve the value of')
    get_cmd.set_defaults(action='get', value=None)
    set_cmd = nested.add_parser('set', help='Store data')
    set_cmd.add_argument('key', help='Key to set')
    set_cmd.add_argument('value', help='Value to store')
    set_cmd.set_defaults(action='set')

    def _unitdata_cmd(action, key, value):
        if action == 'get':
            return unitdata.kv().get(key)
        elif action == 'set':
            unitdata.kv().set(key, value)
            unitdata.kv().flush()
            return ''
    return _unitdata_cmd
