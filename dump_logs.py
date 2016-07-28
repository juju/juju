from argparse import ArgumentParser

from deploy_stack import dump_env_logs
from jujupy import (
    client_from_config,
    )


def main():
    parser = ArgumentParser()
    parser.add_argument('env_name')
    parser.add_argument('directory')
    args = parser.parse_args()

    client = client_from_config(args.env_name, None)
    dump_env_logs(client, None, args.directory)


if __name__ == '__main__':
    main()
