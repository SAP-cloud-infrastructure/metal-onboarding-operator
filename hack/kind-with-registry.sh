#!/usr/bin/env bash
# SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: MIT

set -o errexit
set -o nounset
set -o pipefail

KUBECTL="${KUBECTL:-kubectl}"

KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-metal-onboarding-operator}"
KIND_REGISTRY_PORT="${KIND_REGISTRY_PORT:-5001}"
reg_name="kind-registry-${KIND_CLUSTER_NAME}"

cluster_exists=false
if [[ "$(kind get clusters)" =~ .*"${KIND_CLUSTER_NAME}".* ]]; then
  echo "cluster '${KIND_CLUSTER_NAME}' already exists, skipping create"
  cluster_exists=true
fi

running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run -d --restart=always -p "127.0.0.1:${KIND_REGISTRY_PORT}:5000" --name "${reg_name}" registry:2
fi

if [ "${cluster_exists}" = 'true' ]; then
  if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
    docker network connect "kind" "${reg_name}"
  fi
  exit 0
fi

cat <<EOF | kind create cluster --name "${KIND_CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    config_path = "/etc/containerd/certs.d"
EOF

REGISTRY_DIR="/etc/containerd/certs.d/localhost:${KIND_REGISTRY_PORT}"
node="${KIND_CLUSTER_NAME}-control-plane"
docker exec "${node}" mkdir -p "${REGISTRY_DIR}"
cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${REGISTRY_DIR}/hosts.toml"
[host."http://${reg_name}:5000"]
EOF

if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
  docker network connect "kind" "${reg_name}"
fi

cat <<EOF | "${KUBECTL}" apply --namespace=kube-public -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${KIND_REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

"${KUBECTL}" wait node "${node}" --for=condition=ready --timeout=90s
echo "cluster '${KIND_CLUSTER_NAME}' ready with registry at localhost:${KIND_REGISTRY_PORT}"
