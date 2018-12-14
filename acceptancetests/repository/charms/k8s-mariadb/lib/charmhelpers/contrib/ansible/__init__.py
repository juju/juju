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

# Copyright 2013 Canonical Ltd.
#
# Authors:
#  Charm Helpers Developers <juju@lists.ubuntu.com>
"""Charm Helpers ansible - declare the state of your machines.

This helper enables you to declare your machine state, rather than
program it procedurally (and have to test each change to your procedures).
Your install hook can be as simple as::

    {{{
    import charmhelpers.contrib.ansible


    def install():
        charmhelpers.contrib.ansible.install_ansible_support()
        charmhelpers.contrib.ansible.apply_playbook('playbooks/install.yaml')
    }}}

and won't need to change (nor will its tests) when you change the machine
state.

All of your juju config and relation-data are available as template
variables within your playbooks and templates. An install playbook looks
something like::

    {{{
    ---
    - hosts: localhost
      user: root

      tasks:
        - name: Add private repositories.
          template:
            src: ../templates/private-repositories.list.jinja2
            dest: /etc/apt/sources.list.d/private.list

        - name: Update the cache.
          apt: update_cache=yes

        - name: Install dependencies.
          apt: pkg={{ item }}
          with_items:
            - python-mimeparse
            - python-webob
            - sunburnt

        - name: Setup groups.
          group: name={{ item.name }} gid={{ item.gid }}
          with_items:
            - { name: 'deploy_user', gid: 1800 }
            - { name: 'service_user', gid: 1500 }

      ...
    }}}

Read more online about `playbooks`_ and standard ansible `modules`_.

.. _playbooks: http://www.ansibleworks.com/docs/playbooks.html
.. _modules: http://www.ansibleworks.com/docs/modules.html

A further feature os the ansible hooks is to provide a light weight "action"
scripting tool. This is a decorator that you apply to a function, and that
function can now receive cli args, and can pass extra args to the playbook.

e.g.


@hooks.action()
def some_action(amount, force="False"):
    "Usage: some-action AMOUNT [force=True]"  # <-- shown on error
    # process the arguments
    # do some calls
    # return extra-vars to be passed to ansible-playbook
    return {
        'amount': int(amount),
        'type': force,
    }

You can now create a symlink to hooks.py that can be invoked like a hook, but
with cli params:

# link actions/some-action to hooks/hooks.py

actions/some-action amount=10 force=true

"""
import os
import stat
import subprocess
import functools

import charmhelpers.contrib.templating.contexts
import charmhelpers.core.host
import charmhelpers.core.hookenv
import charmhelpers.fetch


charm_dir = os.environ.get('CHARM_DIR', '')
ansible_hosts_path = '/etc/ansible/hosts'
# Ansible will automatically include any vars in the following
# file in its inventory when run locally.
ansible_vars_path = '/etc/ansible/host_vars/localhost'


def install_ansible_support(from_ppa=True, ppa_location='ppa:rquillo/ansible'):
    """Installs the ansible package.

    By default it is installed from the `PPA`_ linked from
    the ansible `website`_ or from a ppa specified by a charm config..

    .. _PPA: https://launchpad.net/~rquillo/+archive/ansible
    .. _website: http://docs.ansible.com/intro_installation.html#latest-releases-via-apt-ubuntu

    If from_ppa is empty, you must ensure that the package is available
    from a configured repository.
    """
    if from_ppa:
        charmhelpers.fetch.add_source(ppa_location)
        charmhelpers.fetch.apt_update(fatal=True)
    charmhelpers.fetch.apt_install('ansible')
    with open(ansible_hosts_path, 'w+') as hosts_file:
        hosts_file.write('localhost ansible_connection=local ansible_remote_tmp=/root/.ansible/tmp')


def apply_playbook(playbook, tags=None, extra_vars=None):
    tags = tags or []
    tags = ",".join(tags)
    charmhelpers.contrib.templating.contexts.juju_state_to_yaml(
        ansible_vars_path, namespace_separator='__',
        allow_hyphens_in_keys=False, mode=(stat.S_IRUSR | stat.S_IWUSR))

    # we want ansible's log output to be unbuffered
    env = os.environ.copy()
    env['PYTHONUNBUFFERED'] = "1"
    call = [
        'ansible-playbook',
        '-c',
        'local',
        playbook,
    ]
    if tags:
        call.extend(['--tags', '{}'.format(tags)])
    if extra_vars:
        extra = ["%s=%s" % (k, v) for k, v in extra_vars.items()]
        call.extend(['--extra-vars', " ".join(extra)])
    subprocess.check_call(call, env=env)


class AnsibleHooks(charmhelpers.core.hookenv.Hooks):
    """Run a playbook with the hook-name as the tag.

    This helper builds on the standard hookenv.Hooks helper,
    but additionally runs the playbook with the hook-name specified
    using --tags (ie. running all the tasks tagged with the hook-name).

    Example::

        hooks = AnsibleHooks(playbook_path='playbooks/my_machine_state.yaml')

        # All the tasks within my_machine_state.yaml tagged with 'install'
        # will be run automatically after do_custom_work()
        @hooks.hook()
        def install():
            do_custom_work()

        # For most of your hooks, you won't need to do anything other
        # than run the tagged tasks for the hook:
        @hooks.hook('config-changed', 'start', 'stop')
        def just_use_playbook():
            pass

        # As a convenience, you can avoid the above noop function by specifying
        # the hooks which are handled by ansible-only and they'll be registered
        # for you:
        # hooks = AnsibleHooks(
        #     'playbooks/my_machine_state.yaml',
        #     default_hooks=['config-changed', 'start', 'stop'])

        if __name__ == "__main__":
            # execute a hook based on the name the program is called by
            hooks.execute(sys.argv)

    """

    def __init__(self, playbook_path, default_hooks=None):
        """Register any hooks handled by ansible."""
        super(AnsibleHooks, self).__init__()

        self._actions = {}
        self.playbook_path = playbook_path

        default_hooks = default_hooks or []

        def noop(*args, **kwargs):
            pass

        for hook in default_hooks:
            self.register(hook, noop)

    def register_action(self, name, function):
        """Register a hook"""
        self._actions[name] = function

    def execute(self, args):
        """Execute the hook followed by the playbook using the hook as tag."""
        hook_name = os.path.basename(args[0])
        extra_vars = None
        if hook_name in self._actions:
            extra_vars = self._actions[hook_name](args[1:])
        else:
            super(AnsibleHooks, self).execute(args)

        charmhelpers.contrib.ansible.apply_playbook(
            self.playbook_path, tags=[hook_name], extra_vars=extra_vars)

    def action(self, *action_names):
        """Decorator, registering them as actions"""
        def action_wrapper(decorated):

            @functools.wraps(decorated)
            def wrapper(argv):
                kwargs = dict(arg.split('=') for arg in argv)
                try:
                    return decorated(**kwargs)
                except TypeError as e:
                    if decorated.__doc__:
                        e.args += (decorated.__doc__,)
                    raise

            self.register_action(decorated.__name__, wrapper)
            if '_' in decorated.__name__:
                self.register_action(
                    decorated.__name__.replace('_', '-'), wrapper)

            return wrapper

        return action_wrapper
