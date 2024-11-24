#!/bin/bash

pods=()

# Fetches the names of all vault pods
get_vault_pods() {
  po=$(kubectl get pods -n vault --selector=app=vault-server --no-headers -o custom-columns=":metadata.name")
  IFS=$'\n'
  pods=()
  for p in $po; do
    pods+=($p)
  done
}

# Sends SIGHUP to vault process on all vault pods
send_sighup() {
  sighup_script='LOCKFILE="/tmp/watcher.lock"; [ -e "$LOCKFILE" ] && { echo "$HOSTNAME already listening for certificate update."; exit 0; } || { trap "rm -f $LOCKFILE" INT TERM EXIT; touch "$LOCKFILE";file="/vault/userconfig/tls-server/tls.crt"; last_hash=$(md5sum "$file" | awk '"'"'{print $1}'"'"'); while true; do current_hash=$(md5sum "$file" | awk '"'"'{print $1}'"'"'); [ "$current_hash" != "$last_hash" ] && echo "Sending SIGHUP to vault process on $(hostname)" && pkill -SIGHUP vault && break; sleep 1; done }'
  get_vault_pods
  for p in ${pods[@]}; do
    kubectl exec -n vault $p -- /bin/ash -c "$sighup_script" &
  done
}

# Reads the old hash of the secret. Secret name is provided as first arg
get_old_hash() {
  if [ ! -f "/var/lib/watcher/$1" ]; then
    echo ""
  else
    cat "/var/lib/watcher/$1"
  fi
}

# Computes the SHA256 hash of an input value
compute_hash() {
  echo $1 | sha256sum | cut -f 1 -d " "
}

# Writes a hash to the cached secret name hash. Secret name is provided as first
# arg and hash is provided as second
write_hash() {
  echo "$2" > "/var/lib/watcher/$1"
}

# Updates the trust-bundles with the new CA
update_trust_bundle() {
  echo "Updating expiring TLS-CA in trust-bundle"
  kubectl create configmap -n cert-manager expiring-tls-ca --from-literal=root.pem="$(kubectl get configmap -n cert-manager tls-ca -o jsonpath="{['data']['root\.pem']}")" --dry-run=client -o yaml | kubectl apply -f -
  echo "Updating TLS-CA in trust-bundle"
  kubectl create configmap -n cert-manager tls-ca --from-literal=root.pem="$(kubectl get secrets -n vault tls-ca -o jsonpath="{['data']['ca\.crt']}" | base64 -d)" --dry-run=client -o yaml | kubectl apply -f -
  echo "Trust bundle updated"
}


while true
do
  # tls-ca must come first
  secrets=("tls-ca" "tls-server")
  for s in ${secrets[@]}; do
    curr=$(kubectl get secrets -n vault $s -o jsonpath="{.data}")
    if [ -z "$curr" ]; then
      echo "Secret $s unexpectedly empty"
      continue
    fi
    curr_hash=$(compute_hash $curr)
    old_hash=$(get_old_hash "$s")
    if [ "$curr_hash" = "$old_hash" ]; then
      continue
    fi
    echo "Detected changes in $s"
    if [ "$s" = "tls-ca" ]; then
      update_trust_bundle
    else
      # The Vault nodes only store the TLS certificates, not the CA, so if
      # the CA changes, we don't need to send a SIGHUP.
      send_sighup
    fi
    write_hash $s $curr_hash
  done
  sleep 15
done
