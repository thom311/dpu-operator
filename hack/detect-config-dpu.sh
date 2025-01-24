#!/bin/bash

set -e

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

_hostname() {
    printf '%s' "${CDA_CURRENT_HOST:-$(hostname -f)}"
}

CARD_TYPE="$(python ./cluster-deployment-automation/clusterInfo.py -m card-type)" || die "Failure to find cluster info for \"$(_hostname)\""

BASEDIR="$(dirname "$0")"

CONFIG="config-$CARD_TYPE.yaml"

FULL_CONFIG="$BASEDIR/cluster-configs/$CONFIG"

[ -f "$FULL_CONFIG" ] || die "Failed to find configuration file \"$FULL_CONFIG\" for \"$(_hostname)\""

printf '%s\n' "$CONFIG"
