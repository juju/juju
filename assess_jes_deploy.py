#!/usr/bin/env python
from jes import (
    jes_setup,
    check_services,
    deploy_dummy_stack_in_environ,
)
from deploy_stack import (
    deploy_dummy_stack,
    deploy_job_parse_args,
)


def test_jes_deploy(client, charm_prefix, base_env):
    """Deploy the dummy stack in two hosted environments."""

    # deploy into state server
    deploy_dummy_stack(client, charm_prefix)

    # deploy into hosted envs
    deploy_dummy_stack_in_environ(client, charm_prefix, "env1")
    deploy_dummy_stack_in_environ(client, charm_prefix, "env2")

    # check all the services can talk
    check_services(client, base_env)
    check_services(client, "env1")
    check_services(client, "env2")


def main():
    args = deploy_job_parse_args()
    client, charm_prefix, base_env = jes_setup(args)
    test_jes_deploy(client, charm_prefix, base_env)

if __name__ == '__main__':
    main()
