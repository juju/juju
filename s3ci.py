from argparse import ArgumentParser

def parse_args(args):
    parser = ArgumentParser()
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    parser_get_juju_bin = subparsers.add_parser(
        'get-juju-bin', help='Retrieve and extract juju binaries.')
    parser_get_juju_bin.add_argument('config')
    parser_get_juju_bin.add_argument('revision_build')
    parser_get_juju_bin.add_argument('workspace', nargs='?', default='.')
    return parser.parse_args(args)
