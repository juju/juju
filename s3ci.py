from argparse import ArgumentParser
from boto.s3.connection import S3Connection
from ConfigParser import ConfigParser
import sys


def parse_args(args=None):
    parser = ArgumentParser()
    subparsers = parser.add_subparsers(help='sub-command help', dest='command')
    parser_get_juju_bin = subparsers.add_parser(
        'get-juju-bin', help='Retrieve and extract juju binaries.')
    parser_get_juju_bin.add_argument('config')
    parser_get_juju_bin.add_argument('revision_build', type=int)
    parser_get_juju_bin.add_argument('workspace', nargs='?', default='.')
    return parser.parse_args(args)


def get_s3_credentials(s3cfg_path):
    config = ConfigParser()
    config.read(s3cfg_path)
    access_key = config.get('default', 'access_key')
    secret_key = config.get('default', 'secret_key')
    return access_key, secret_key


def get_juju_bin(bucket, revision_build, workspace):
    pass


def main():
    args = parse_args()
    credentials = get_s3_credentials(args.config)
    conn = S3Connection(*credentials)
    bucket = conn.get_bucket('juju-qa-data')
    get_juju_bin(bucket, args.revision_build, args.workspace)


if __name__ == '__main__':
    sys.exit(main())
