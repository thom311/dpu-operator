#!/usr/bin/env bash

set -e
set -x

die() {
    printf "%s\n" "$*"
    exit 1
}

cd "$(dirname "$0")/.."

source ./hack/_source_common.sh

CLUSTER_KIND="$1"

DPU_KIND="$(detect_dpu_kind)"

CLUSTER_CONFIG=
case "$CLUSTER_KIND" in
        "dpu")
            case "$DPU_KIND" in
                dpu)
                    CLUSTER_CONFIG="config-dpu.yaml"
                    ;;
            esac
            ;;
        "dpu-host")
            case "$DPU_KIND" in
                dpu)
                    CLUSTER_CONFIG="config-dpu-host.yaml"
                    ;;
            esac
            ;;
esac

test -n "$CLUSTER_CONFIG" || die "Could not detect cluster config for CLUSTER_KIND=\"$CLUSTER_KIND\" and DPU_KIND=\"$DPU_KIND\""

CLUSTER_CONFIG_PATH="hack/cluster-configs/$CLUSTER_CONFIG"
test -f "$CLUSTER_CONFIG_PATH"

source ./hack/_source_python_venv.sh

pushd cluster-deployment-automation
    pip install -r requirements.txt
    ret=0
    python cda.py -v debug --secret /root/pull_secret.json "../$CLUSTER_CONFIG_PATH" deploy || ret=$?
popd

if [ "$ret" != 0 ]; then
    echo "cluster-deployment-automation deployment of $CLUSTER_CONFIG failed with error code $ret"
    exit "$ret"
fi

case "$CLUSTER_KIND" in
    "dpu")
        KUBECONFIG="/root/kubeconfig.microshift"
        ;;
    "dpu-host")
        KUBECONFIG="/root/kubeconfig.ocpcluster"
        ;;
esac
export KUBECONFIG

oc -n openshift-dpu-operator wait --for=condition=Ready pod --all --timeout=3m || :
oc -n openshift-dpu-operator get all

echo "Successfully Deployed Cluster $CLUSTER_CONFIG"
