#!/usr/bin/env bash

set -e

git submodule init
git submodule update

dnf install python3.11 -y

rm -rf ./.tmp/ocp-venv

source ./hack/_source_python_venv.sh

pushd cluster-deployment-automation
sh ./dependencies.sh
popd

pushd ocp-traffic-flow-tests
pip install -r requirements.txt
popd
