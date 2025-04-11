#!/bin/bash

set -e

cd "$(dirname "$0")/.."

export FAST_TEST=true
export NF_INGRESS_IP=10.20.30.2
export EXTERNAL_CLIENT_DEV=eno12409
export EXTERNAL_CLIENT_IP=10.20.30.100
export KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.27.1 --bin-dir bin -p path)"

./bin/ginkgo -coverprofile cover.out ./e2e_test/...
