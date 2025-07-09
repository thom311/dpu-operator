#!/bin/bash

set -e

C_YELLOW=$'\033[33m'
C_RED=$'\033[1;31m'
C_GREEN=$'\033[1;32m'
C_DARK=$'\033[90m'
C_RESET=$'\033[0m'

_echo() {
    printf '%s' "$*"
}

_echo_nl() {
    printf '%s\n' "$*"
}

die() {
    _echo_nl "ERROR: ${C_RED}$*${C_RESET}"
    exit 1
}

cd "$(dirname "$0")/.."

_cmd() {
    local out

    _echo "${C_GREEN}"
    printf "%q " "$@"
    _echo "${C_RESET}"
    printf "\n"

    local rc=0
    out="$("$@" 2>&1)" || rc="$?"

    _echo_nl "${C_DARK}$out${C_RESET}" | sed 's/^/    /' 1>&2

    if [ "$MAY_FAIL" != 1 ] && [ "$rc" != 0 ] ; then
        die "Command failed"
    fi
}

_pull() {
    local img="$1"
    local arch="$2"
    local tag="$3"

    _cmd podman pull --platform="$arch" "$img"
    _cmd podman tag "$img" "$tag"
}

_pull_and_push() {
    local TMP_PREFIX
    local SRC="$1"
    local DST="$2"
    TMP_PREFIX="tmp-repush-image-$(_echo "$SRC" | sed 's/[^a-zA-Z0-9_]/_/g')"
    local TMP_SRC_AMD="$TMP_PREFIX-amd"
    local TMP_SRC_ARM="$TMP_PREFIX-arm"
    local TMP_MANIFEST="localhost/$TMP_PREFIX-manifest"

    if [[ "$DST" != *"/"* ]] ; then
        DST="$DST/${SRC#*/}"
    fi

    _echo_nl "## ${C_YELLOW}$SRC${C_RESET} -> ${C_YELLOW}$DST${C_RESET}"

    _pull "$SRC" linux/amd64 "$TMP_SRC_AMD"
    _pull "$SRC" linux/arm64 "$TMP_SRC_ARM"

    MAY_FAIL=1 _cmd buildah manifest rm "$TMP_MANIFEST"
    _cmd buildah manifest create "$TMP_MANIFEST"
    _cmd buildah manifest add "$TMP_MANIFEST" "containers-storage:localhost/$TMP_SRC_ARM"
    _cmd buildah manifest add "$TMP_MANIFEST" "containers-storage:localhost/$TMP_SRC_AMD"

    # shellcheck disable=SC2086
    _cmd buildah manifest push $PUSH_ARGS --all "$TMP_MANIFEST" "docker://$DST"

    _echo_nl
}

DST="$1"
[ -z "$DST" ] && DST="$MIRROR_REGISTRY"
[ -n "$DST" ] || die "Missing mirror registry. Either set as first argument or via \$MIRROR_REGISTRY"
shift || :

srcs=( "$@" )
if [ "${#srcs[@]}" -eq 0 ] ; then
    mapfile -t srcs <<< "$(sed -n 's/FROM \+\(registry.ci.openshift.org\/[^ ]\+\).*/\1/p' Dockerfile.* | sort -u)"
fi

MAY_FAIL=0
for SRC in "${srcs[@]}" ; do
    _pull_and_push "$SRC" "$DST"
done
