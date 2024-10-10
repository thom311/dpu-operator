#!/usr/bin/env bash

set -e
set -x

cd "$(dirname "$0")/.."

CLUSTER_CONFIG="$1"
test -n "$CLUSTER_CONFIG"

CLUSTER_CONFIG_PATH="hack/cluster-configs/$CLUSTER_CONFIG"
test -f "$CLUSTER_CONFIG_PATH"

python3.11 -m venv ./.tmp/ocp-venv
source ./.tmp/ocp-venv/bin/activate

pushd cluster-deployment-automation
    pip install -r requirements.txt
    ret=0
    python cda.py --secret /root/pull_secret.json "../$CLUSTER_CONFIG_PATH" deploy || ret=$?
popd

if [ "$ret" = 0 ]; then
    echo "Successfully Deployed Cluster $CLUSTER_CONFIG"
else
    echo "cluster-deployment-automation deployment of $CLUSTER_CONFIG failed with error code $ret"
fi
exit "$ret"
