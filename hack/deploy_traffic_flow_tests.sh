#!/usr/bin/env bash

set -e

source ./.tmp/ocp-venv/bin/activate

cd ocp-traffic-flow-tests

export KUBECONFIG=/root/kubeconfig.ocpcluster
nodes=$(oc get nodes)
export worker=$(echo "$nodes" | grep -oP '^worker-[^\s]*')


# wa for https://issues.redhat.com/browse/IIC-364
pushd ../
make undeploy
make local-deploy
oc create -f examples/host.yaml
sleep 45 # Give times for Intel VSP to configure ip on <ipu-netdev>d3
popd

export KUBECONFIG=/root/kubeconfig.microshift
nodes=$(oc get nodes)
export acc=$(echo "$nodes" | grep -oP '^\d{3}-acc')

envsubst < ../hack/cluster-configs/ocp-tft-config.yaml > tft_config.yaml

python3.11 main.py tft_config.yaml
