#!/bin/bash

# This example script uses the grpc gateway to interact with the controller

# replace the following with you user's details
USERNAME=admin
PASSWORD=password

# That can be found in ~/.local/share/juju/controllers.yaml, field api-endpoints
# of a controller
CONTROLLER_IP=10.225.205.93

# That can be found in ~/.local/share/juju/controllers.yaml, field ca-cert of a
# controller
CACERT=cacert.pem

# That can be found in ~/.local/share/juju/models.yaml field uuid of a model
MODEL_UUID=2753b34b-a8cf-4a09-8413-2b6b5cec510e

function callAPI {
    local endpoint=$1
    shift
    curl -s --resolve "juju-apiserver:18889:$CONTROLLER_IP" --cacert "$CACERT" \
        -H "Authorization: basic $USERNAME:$PASSWORD" \
        -H "X-Juju-Model-Uuid: $MODEL_UUID" \
        -H "Content-Type: application/json" \
        https://juju-apiserver:18889/"$endpoint" "$@" | jq
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
