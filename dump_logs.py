from argparse import ArgumentParser

from deploy_stack import dump_env_logs
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )


def main():
    parser = ArgumentParser()
    parser.add_argument('env_name')
    parser.add_argument('directory')
    args = parser.parse_args()

    env = SimpleEnvironment.from_config(args.env_name)
    client = EnvJujuClient.by_version(env)
    dump_env_logs(client, None, args.directory)


if __name__ == '__main__':
    main()
