#!/usr/bin/env python
__metaclass__ = type


from jujupy import Environment, check_wordpress

import sys


def deploy_stack(environments):
    """"Deploy a Wordpress stack in the specified environment.

    :param environment: The name of the desired environment.
    """
    envs = [Environment(e) for e in environments]
    for env in envs:
        env.juju('bootstrap', '--constraints', 'mem=2G')
    for env in envs:
        env.juju('deploy', 'wordpress')
        env.juju('deploy', 'mysql')
        env.juju('add-relation', 'mysql', 'wordpress')
        env.juju('expose', 'wordpress')
    for env in envs:
        status = env.wait_for_started()
        wp_unit_0 = status['services']['wordpress']['units']['wordpress/0']
        check_wordpress(wp_unit_0['public-address'])


def main():
    try:
        deploy_stack(sys.argv[1:])
    except Exception as e:
        print e
        sys.exit(1)


if __name__ == '__main__':
    main()
