#!/usr/bin/env bash

set -e

source /tmp/cda-venv/bin/activate

CONFIG="$(./hack/detect-config-dpu.sh)"

cd cluster-deployment-automation

./cda.py --secret /root/pull_secret.json ../hack/cluster-configs/"$CONFIG" deploy -s post

ret=$?
if [ $ret == 0 ]; then
    echo "Successfully Deployed ISO cluster's post config"
else
    echo "cluster-deployment-automation post config deployment failed with error code $ret"
    exit $ret
fi
