#!/bin/bash

# This example script uses the grpc gateway to interact with the controller.
# You can use the setup.sh script in the parent directory to set up the
# HTTP_API_ADDR, CACERT_PATH, MODEL_UUID variables automatically from
# ~/.local/share/juju.

# replace the following with your user's details
USERNAME=admin
PASSWORD=password

# That can be found in ~/.local/share/juju/controllers.yaml, field api-endpoints
# of a controller (but the port is 17073)
SERVER_HOST=${HTTP_API_ADDR%:*}
SERVER_PORT=${HTTP_API_ADDR#*:}
# SERVER_HOST=10.225.205.93
# SERVER_PORT=17073

# That can be found in ~/.local/share/juju/controllers.yaml, field ca-cert of a
# controller
# CACERT_PATH=cacert.pem

# That can be found in ~/.local/share/juju/models.yaml field uuid of a model
# MODEL_UUID=1beb1568-52a3-45ed-8b0b-5e0989f0882c


function callAPI {
    local endpoint=$1
    shift
    curl -s --resolve "juju-apiserver:$SERVER_PORT:$SERVER_HOST" --cacert "$CACERT_PATH" \
        -H "Authorization: basic $USERNAME:$PASSWORD" \
        -H "X-Juju-Model-Uuid: $MODEL_UUID" \
        -H "Content-Type: application/json" \
        https://juju-apiserver:$SERVER_PORT/"$endpoint" "$@" | jq
}


echo "Requesting the model status from the controller"
callAPI v1alpha1/status

echo "Requesting deploment of the 'postgresql' charm"
callAPI v1alpha1/deploy -X POST -d '{"charm_name": "postgresql"}'

# To remove application instead:

# echo "Requesting removal of the 'postgresql' application"
# callAPI v1alpha1/removeApplication -X POST -d '{"application_name": "postgresql"}'

echo "Waiting for 5 seconds..."
sleep 5

echo "Requesting the model status again"
callAPI v1alpha1/status
