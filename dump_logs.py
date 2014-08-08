from argparse import ArgumentParser

from deploy_stack import dump_logs
from jujupy import Environment

def main():
    parser = ArgumentParser()
    parser.add_argument('env_name')
    parser.add_argument('directory')
    args = parser.parse_args()

    env = Environment.from_config(args.env_name)
    status = env.get_status()
    for machine_id, machine in status.iter_machines():
        print machine_id
        dump_logs(env, machine['dns-name'], args.directory)

if __name__ == '__main__':
    main()
