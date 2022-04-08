import argparse
import grpc
from juju.client.simple.v1alpha1.simple_pb2_grpc import SimpleServiceStub
import juju.client.simple.v1alpha1.simple_pb2 as simpleapi


parser = argparse.ArgumentParser()

parser.add_argument('--controller', type=str)
parser.add_argument('--username', type=str)
parser.add_argument('--password', type=str)
parser.add_argument('--model', type=str)
parser.add_argument('--cacert', default="cacert.pem")

# Choose you action with these flags
parser.add_argument('--status', action='store_true')
parser.add_argument('--deploy', action="store_true")
parser.add_argument('--remove', action="store_true")

args = parser.parse_args()

with open(args.cacert, 'rb') as f:
    creds = grpc.ssl_channel_credentials(f.read())

channel = grpc.secure_channel(args.controller + ":18888", creds, (
    ('grpc.ssl_target_name_override', 'juju-apiserver'),
))

client = SimpleServiceStub(channel)

# That probably should be injected with some middleware but no time to work out
# how it's done.
metadata = (
    ('authorization', f'basic {args.username}:{args.password}'),
    ('model-uuid', args.model),
)

if args.status:
    resp = client.Status(
        request=simpleapi.StatusRequest(),
        metadata=metadata
    )
elif args.deploy:
    resp = client.Deploy(
        request=simpleapi.DeployRequest(charm_name="postgresql"),
        metadata=metadata
    )
elif args.remove:
    resp = client.RemoveApplication(
        request=simpleapi.RemoveApplicationRequest(application_name="postgresql"),
        metadata=metadata
    )
else:
    resp = "please choose --status, --deploy, --remove"

print(resp)
