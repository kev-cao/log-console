#!/bin/bash

set -euo pipefail

MASTER="k3sMaster"
NUM_NODES=3

NODE () {
  echo "k3sWorker$1"
}
NUM_WORKERS=$(($NUM_NODES - 1))

# Launch nodes
pids=()
multipass launch --name $MASTER --cpus 2 --memory 4G --disk 20G &
pids[0]=$!
for i in $(seq $NUM_WORKERS);
do
  multipass launch --name $(NODE $i) --cpus 2 --memory 4G --disk 20G &
  pids[$(($i + 1))]=$!
done
for pid in ${pids[*]}; do
  wait $pid
done

# Launch K3S on master node and get connection info
multipass exec $MASTER -- /bin/bash -c "curl -sfL https://get.k3s.io | K3S_KUBECONFIG_MODE=644 sh -"
K3S_MASTER_IP="https://$(multipass info $MASTER | grep 'IPv4' | awk -F' ' '{print $2}'):6443"
MASTER_TOKEN="$(multipass exec $MASTER -- /bin/bash -c "sudo cat /var/lib/rancher/k3s/server/node-token")"
# Set up K3S agents on workers
pids=()
for i in $(seq $NUM_WORKERS);
do
  multipass exec $(NODE $i) -- /bin/bash -c "curl -sfL https://get.k3s.io | K3S_TOKEN=${MASTER_TOKEN} K3S_URL=${K3S_MASTER_IP} sh -" &
  pids[$i]=$!
done
for pid in ${pids[*]}; do
  wait $pid
done

# Mount app repo to master node
SCRIPT_DIR=$(dirname $0)
multipass mount --type=classic $SCRIPT_DIR $MASTER:/home/ubuntu/log-console

kubeCfg="KUBECONFIG=/etc/rancher/k3s/k3s.yaml"

# Install Helm
multipass exec $MASTER -- /bin/bash -c "curl -fsSL -o install-helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 && chmod u+x install-helm.sh && ./install-helm.sh"
multipass exec $MASTER -- /bin/bash -c "helm repo add hashicorp https://helm.releases.hashicorp.com"
multipass exec $MASTER -- /bin/bash -c "helm repo update"

# Install Cert-manager
multipass exec $MASTER -- /bin/bash -c "kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.3/cert-manager.yaml"
multipass exec $MASTER -- /bin/bash -c "sudo apt install golang-go --yes"
multipass exec $MASTER -- /bin/bash -c "go install github.com/cert-manager/cmctl/v2@latest"
multipass exec $MASTER -- /bin/bash -c "$kubeCfg \$(go env GOPATH)/bin/cmctl check api --wait=2m"

# Set up Vault
multipass exec $MASTER -- /bin/bash -c "kubectl create namespace vault"
## Set up certificates
multipass exec $MASTER -- /bin/bash -c "kubectl apply -f ~/log-console/k8s/cert-manager.yaml"
multipass exec $MASTER -- /bin/bash -c \
  "$kubeCfg helm install vault hashicorp/vault \
  -f ~/log-console/k8s/vault-overrides.yaml \
  --namespace vault"

