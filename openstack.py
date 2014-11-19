import sys

from jujupy import SimpleEnvironment
from substrate import OpenStackAccount

def main():
    new_env = SimpleEnvironment.from_config('test-release-hp')
    substrate = OpenStackAccount.from_config(new_env.config)
    groups = substrate.list_instance_security_groups()
    print groups

if __name__ == '__main__':
    sys.exit(main())

