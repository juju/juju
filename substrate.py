import os
import subprocess

from jujuconfig import (
    get_euca_env,
    translate_to_env,
    )
from utility import print_now


def terminate_instances(env, instance_ids):
    provider_type = env.config.get('type')
    environ = dict(os.environ)
    if provider_type == 'ec2':
        environ.update(get_euca_env(env.config))
        command_args = ['euca-terminate-instances'] + instance_ids
    elif provider_type == 'openstack':
        environ.update(translate_to_env(env.config))
        command_args = ['nova', 'delete'] + instance_ids
    else:
        raise ValueError(
            "This test does not support the %s provider" % provider_type)
    if len(instance_ids) == 0:
        print_now("No instances to delete.")
        return
    print_now("Deleting %s." % ', '.join(instance_ids))
    subprocess.check_call(command_args, env=environ)
