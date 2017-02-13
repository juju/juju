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
from charmhelpers.contrib.benchmark import Benchmark


@cmdline.subcommand(command_name='benchmark-start')
def start():
    Benchmark.start()


@cmdline.subcommand(command_name='benchmark-finish')
def finish():
    Benchmark.finish()


@cmdline.subcommand_builder('benchmark-composite', description="Set the benchmark composite score")
def service(subparser):
    subparser.add_argument("value", help="The composite score.")
    subparser.add_argument("units", help="The units the composite score represents, i.e., 'reads/sec'.")
    subparser.add_argument("direction", help="'asc' if a lower score is better, 'desc' if a higher score is better.")
    return Benchmark.set_composite_score
