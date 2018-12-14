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

"""Charm Helpers saltstack - declare the state of your machines.

This helper enables you to declare your machine state, rather than
program it procedurally (and have to test each change to your procedures).
Your install hook can be as simple as::

    {{{
    from charmhelpers.contrib.saltstack import (
        install_salt_support,
        update_machine_state,
    )


    def install():
        install_salt_support()
        update_machine_state('machine_states/dependencies.yaml')
        update_machine_state('machine_states/installed.yaml')
    }}}

and won't need to change (nor will its tests) when you change the machine
state.

It's using a python package called salt-minion which allows various formats for
specifying resources, such as::

    {{{
    /srv/{{ basedir }}:
        file.directory:
            - group: ubunet
            - user: ubunet
            - require:
                - user: ubunet
            - recurse:
                - user
                - group

    ubunet:
        group.present:
            - gid: 1500
        user.present:
            - uid: 1500
            - gid: 1500
            - createhome: False
            - require:
                - group: ubunet
    }}}

The docs for all the different state definitions are at:
    http://docs.saltstack.com/ref/states/all/


TODO:
  * Add test helpers which will ensure that machine state definitions
    are functionally (but not necessarily logically) correct (ie. getting
    salt to parse all state defs.
  * Add a link to a public bootstrap charm example / blogpost.
  * Find a way to obviate the need to use the grains['charm_dir'] syntax
    in templates.
"""
# Copyright 2013 Canonical Ltd.
#
# Authors:
#  Charm Helpers Developers <juju@lists.ubuntu.com>
import subprocess

import charmhelpers.contrib.templating.contexts
import charmhelpers.core.host
import charmhelpers.core.hookenv


salt_grains_path = '/etc/salt/grains'


def install_salt_support(from_ppa=True):
    """Installs the salt-minion helper for machine state.

    By default the salt-minion package is installed from
    the saltstack PPA. If from_ppa is False you must ensure
    that the salt-minion package is available in the apt cache.
    """
    if from_ppa:
        subprocess.check_call([
            '/usr/bin/add-apt-repository',
            '--yes',
            'ppa:saltstack/salt',
        ])
        subprocess.check_call(['/usr/bin/apt-get', 'update'])
    # We install salt-common as salt-minion would run the salt-minion
    # daemon.
    charmhelpers.fetch.apt_install('salt-common')


def update_machine_state(state_path):
    """Update the machine state using the provided state declaration."""
    charmhelpers.contrib.templating.contexts.juju_state_to_yaml(
        salt_grains_path)
    subprocess.check_call([
        'salt-call',
        '--local',
        'state.template',
        state_path,
    ])
