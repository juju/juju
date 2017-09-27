#!/usr/bin/env python
from argparse import ArgumentParser
from substrate import stop_libvirt_domain


def main():
    parser = ArgumentParser()
    parser.add_argument('--URI', help='Hypervisor URI',
                        default='qemu+ssh://localhost/system')
    parser.add_argument('domain', help='The name of the libvirt domain to '
                        'stop.')
    args = parser.parse_args()
    print("Attempting to stop %s at %s" % (args.domain, args.URI))
    status_msg = stop_libvirt_domain(args.URI, args.domain)
    print("%s" % status_msg)


if __name__ == '__main__':
    main()
