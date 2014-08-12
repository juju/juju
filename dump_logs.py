from argparse import ArgumentParser

from deploy_stack import dump_env_logs
from jujupy import Environment


def main():
    parser = ArgumentParser()
    parser.add_argument('env_name')
    parser.add_argument('directory')
    args = parser.parse_args()

    env = Environment.from_config(args.env_name)
    dump_env_logs(env, None, args.directory)

if __name__ == '__main__':
    main()
