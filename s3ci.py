from argparse import ArgumentParser
from ConfigParser import ConfigParser


def parse_args(args):
    parser = ArgumentParser()
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    parser_get_juju_bin = subparsers.add_parser(
        'get-juju-bin', help='Retrieve and extract juju binaries.')
    parser_get_juju_bin.add_argument('config')
    parser_get_juju_bin.add_argument('revision_build')
    parser_get_juju_bin.add_argument('workspace', nargs='?', default='.')
    return parser.parse_args(args)


def get_s3_credentials(s3cfg_path):
    config = ConfigParser()
    config.read(s3cfg_path)
    access_key = config.get('default', 'access_key')
    secret_key = config.get('default', 'secret_key')
    return access_key, secret_key
