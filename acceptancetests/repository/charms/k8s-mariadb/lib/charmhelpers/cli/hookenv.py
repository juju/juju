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
from charmhelpers.core import hookenv


cmdline.subcommand('relation-id')(hookenv.relation_id._wrapped)
cmdline.subcommand('service-name')(hookenv.service_name)
cmdline.subcommand('remote-service-name')(hookenv.remote_service_name._wrapped)
