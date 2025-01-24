#!/usr/bin/env bash

set -e

source /tmp/cda-venv/bin/activate

CONFIG="$(./hack/detect-config-dpu.sh)"

cd cluster-deployment-automation

# Tear down any previous cluster fully
./cda.py --secret /root/pull_secret.json ../hack/cluster-configs/config-dpu-host.yaml deploy -f

parallel -u --halt 2 ::: \
  "./cda.py --secret /root/pull_secret.json ../hack/cluster-configs/"$CONFIG" deploy && echo 'Successfully Deployed ISO Cluster'" \
  "./cda.py --secret /root/pull_secret.json ../hack/cluster-configs/config-dpu-host.yaml deploy --steps pre,masters && echo 'Successfully Deployed DPU host Cluster'"

./cda.py --secret /root/pull_secret.json ../hack/cluster-configs/config-dpu-host.yaml deploy --steps workers,post && echo "Successfully Deployed DPU host Cluster"
