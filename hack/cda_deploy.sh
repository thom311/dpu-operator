#!/usr/bin/env bash

set -e
set -x

cd "$(dirname "$0")/.."

CLUSTER_CONFIG="$1"
test -n "$CLUSTER_CONFIG"

CLUSTER_CONFIG_PATH="hack/cluster-configs/$CLUSTER_CONFIG"
test -f "$CLUSTER_CONFIG_PATH"

source ./hack/_source_python_venv.sh

pushd cluster-deployment-automation
    pip install -r requirements.txt
    ret=0
    python cda.py -v debug --secret /root/pull_secret.json "../$CLUSTER_CONFIG_PATH" deploy || ret=$?
popd

if [ "$ret" = 0 ]; then
    echo "Successfully Deployed Cluster $CLUSTER_CONFIG"
else
    echo "cluster-deployment-automation deployment of $CLUSTER_CONFIG failed with error code $ret"
fi
exit "$ret"
