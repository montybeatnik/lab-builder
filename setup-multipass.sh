#!/usr/bin/env bash
set -Eeuo pipefail
trap 'echo "ERROR on line $LINENO. Exiting." >&2' ERR

# Go toolchain — archive is selected dynamically from VM architecture.
GO_VERSION="1.23.1"

### ───────────────────────────
### EDIT THESE FOR YOUR SETUP
### ───────────────────────────
VM_NAME="lab-builder"
UBUNTU_RELEASE="jammy"          # or "22.04"
CPUS="6"
MEM="12G"
DISK="60G"

# Repo root defaults to this script's directory; can be overridden by env.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="${REPO_DIR:-$SCRIPT_DIR}"

# Optional path to a cEOS-lab tarball on host. If missing, FRR labs still work.
CEOS_TARBALL="${CEOS_TARBALL:-$REPO_DIR/cEOSarm-lab-4.34.2.1F.tar.xz}"

# How you want the Docker image tagged inside the VM
CEOS_TAG="ceosimage:4.34.2.1f"

# Set to 1 to auto-deploy the lab at the end, 0 to skip
AUTO_DEPLOY="${AUTO_DEPLOY:-0}"
LAB_FILE="lab.clab.yml"         # path inside your REPO_DIR

### ───────────────────────────
### helpers
### ───────────────────────────
err()  { echo "ERROR: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "Missing dependency '$1' on host."; }

ensure_multipassd() {
  # If the daemon/socket isn’t ready, try to start it without failing the whole run.
  if ! multipass list >/dev/null 2>&1; then
    echo ">>> Starting multipassd..."
    if command -v launchctl >/dev/null 2>&1; then
      sudo launchctl kickstart -k system/com.canonical.multipassd || true
    fi
    # Open the app as a fallback (often triggers required macOS prompts)
    open -ga "Multipass.app" >/dev/null 2>&1 || true
    sleep 2
    multipass list >/dev/null 2>&1 || err "Cannot connect to multipassd. Please open Multipass.app once and re-run."
  fi
}

mounted_repo() {
  multipass info "$VM_NAME" 2>/dev/null | grep -q "/home/ubuntu/lab"
}

### ───────────────────────────
### sanity checks
### ───────────────────────────
need multipass
ensure_multipassd

[[ -d "$REPO_DIR" ]] || err "REPO_DIR not found: $REPO_DIR"
[[ -f "$REPO_DIR/$LAB_FILE" ]] || err "Lab file not found at: $REPO_DIR/$LAB_FILE"

echo ">>> Using repo:       $REPO_DIR"
if [[ -f "$CEOS_TARBALL" ]]; then
  echo ">>> Using cEOS image: $CEOS_TARBALL  ->  $CEOS_TAG"
else
  echo ">>> cEOS image not found at $CEOS_TARBALL (continuing; FRR-only labs will work)"
fi

### ───────────────────────────
### launch (or reuse) multipass VM
### ───────────────────────────
if multipass info "$VM_NAME" >/dev/null 2>&1; then
  echo ">>> VM '$VM_NAME' already exists. Reusing."
else
  echo ">>> Launching VM '$VM_NAME' ($UBUNTU_RELEASE, $CPUS vCPU, $MEM RAM, $DISK disk)..."
  multipass launch "$UBUNTU_RELEASE" --name "$VM_NAME" --cpus "$CPUS" --memory "$MEM" --disk "$DISK"
fi

echo ">>> Waiting for cloud-init to finish..."
multipass exec "$VM_NAME" -- cloud-init status --wait

### ───────────────────────────
### mount repo & transfer cEOS tarball
### ───────────────────────────
if mounted_repo; then
  echo ">>> Repo already mounted at ~/lab"
else
  echo ">>> Mounting repo -> $VM_NAME:~/lab"
  multipass mount "$REPO_DIR" "$VM_NAME":/home/ubuntu/lab
fi

if [[ -f "$CEOS_TARBALL" ]]; then
  echo ">>> Ensuring ~/images exists and transferring cEOS tarball"
  multipass exec "$VM_NAME" -- bash -lc "mkdir -p ~/images"
  multipass transfer "$CEOS_TARBALL" "$VM_NAME":/home/ubuntu/images/
fi

### ───────────────────────────
### install docker, containerlab, and tools
### ───────────────────────────
echo ">>> Installing Docker, Containerlab, and utilities inside the VM..."
multipass exec "$VM_NAME" -- bash -lc '
  set -euo pipefail
  sudo apt-get update
  sudo apt-get install -y ca-certificates curl gnupg lsb-release jq iproute2 arping iperf3

  # Docker (convenience script)
  if ! command -v docker >/dev/null 2>&1; then
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker ubuntu || true
  fi

  # Containerlab
  if ! command -v containerlab >/dev/null 2>&1; then
    curl -sL https://get.containerlab.dev | bash
  fi

  # Helpful sysctls (not strictly required for clab, but useful)
  echo "net.ipv4.ip_forward=1" | sudo tee /etc/sysctl.d/99-clab.conf >/dev/null
  sudo sysctl --system >/dev/null
'

### ───────────────────────────
### import cEOS image (ARM64), verify arch
### ───────────────────────────
if [[ -f "$CEOS_TARBALL" ]]; then
  echo ">>> Importing cEOS Docker image as $CEOS_TAG ..."
  multipass exec "$VM_NAME" -- bash -lc "
    set -euo pipefail
    ls -lh ~/images
    if ! sudo docker image inspect '$CEOS_TAG' >/dev/null 2>&1; then
      sudo docker import ~/images/$(basename "$CEOS_TARBALL") '$CEOS_TAG'
    else
      echo 'Docker image $CEOS_TAG already present, skipping import.'
    fi

    echo '>>> Verifying image arch...'
    IMG_ARCH=\$(sudo docker inspect '$CEOS_TAG' --format '{{.Architecture}}')
    HOST_ARCH=\$(uname -m)
    echo \"Image arch: \$IMG_ARCH | Host arch: \$HOST_ARCH\"
    if [[ \"\$IMG_ARCH\" != \"arm64\" && \"\$IMG_ARCH\" != \"aarch64\" ]]; then
      echo 'ERROR: Imported image is not ARM64. Please import an ARM64 cEOS tarball.' >&2
      exit 1
    fi
  "
fi

### ───────────────────────────
### fix clab labdir ACL (don’t write on mount)
### ───────────────────────────
multipass exec "$VM_NAME" -- bash -lc '
  mkdir -p ~/.clab-runs
  if ! grep -q CLAB_LABDIR_BASE ~/.bashrc; then
    echo "export CLAB_LABDIR_BASE=\$HOME/.clab-runs" >> ~/.bashrc
  fi
'

### ───────────────────────────
### show versions & repo path
### ───────────────────────────
multipass exec "$VM_NAME" -- bash -lc '
  echo ">>> Versions:"
  docker --version
  containerlab version || true
  echo
  echo ">>> Repo is mounted at: ~/lab"
  ls -la ~/lab
'

### ───────────────────────────
### ensure gNMIc image matches VM arch
### ───────────────────────────
echo ">>> Pre-pulling gNMIc image for VM architecture..."
multipass exec "$VM_NAME" -- bash -lc '
  set -euo pipefail
  image="ghcr.io/openconfig/gnmic:latest"
  arch="$(uname -m)"
  if [[ "$arch" == "arm64" || "$arch" == "aarch64" ]]; then
    sudo docker pull --platform=linux/arm64 "$image"
  else
    sudo docker pull "$image"
  fi
'

### ───────────────────────────
### optional: deploy the lab
### ───────────────────────────
if [[ "$AUTO_DEPLOY" -eq 1 ]]; then
  echo ">>> Deploying lab from ~/lab/$LAB_FILE ..."
  multipass exec "$VM_NAME" -- bash -lc "
    set -euo pipefail
    cd ~/lab
    # ensure env var is seen by sudo
    export CLAB_LABDIR_BASE=\$HOME/.clab-runs
    sudo -E containerlab destroy -t '$LAB_FILE' || true
    sudo -E containerlab deploy -t '$LAB_FILE' --reconfigure
    echo
    echo '>>> Lab status:'
    containerlab inspect -t '$LAB_FILE' | sed -n '1,120p'
  "
else
  cat <<EOF

>>> Skipping auto-deploy (AUTO_DEPLOY=0).
To deploy manually:

  multipass shell $VM_NAME
  cd ~/lab
  export CLAB_LABDIR_BASE=\$HOME/.clab-runs
  sudo -E containerlab deploy -t $LAB_FILE --reconfigure

EOF
fi

echo ">>> Installing Go ${GO_VERSION} in the VM..."
multipass exec "$VM_NAME" -- bash -lc '
  set -euo pipefail
  sudo apt-get update
  # Useful build tools + git
  sudo apt-get install -y git build-essential

  # If the requested version is already present, skip reinstall
  if [[ -x /usr/local/go/bin/go ]]; then
    CUR=$(/usr/local/go/bin/go version | awk "{print \$3}" || true)
  else
    CUR=""
  fi

  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64) GO_ARCH="amd64" ;;
    arm64|aarch64) GO_ARCH="arm64" ;;
    *) echo "Unsupported VM architecture: $ARCH" >&2; exit 1 ;;
  esac
  GO_TGZ="go'"$GO_VERSION"'.linux-${GO_ARCH}.tar.gz"

  if [[ "$CUR" != "go'"$GO_VERSION"'" ]]; then
    echo "Downloading Go '"$GO_VERSION"' for ${GO_ARCH}..."
    curl -fsSL "https://go.dev/dl/${GO_TGZ}" -o "/tmp/${GO_TGZ}"
    echo "Installing to /usr/local/go ..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${GO_TGZ}"
    rm -f "/tmp/${GO_TGZ}"
  else
    echo "Go '"$GO_VERSION"' already installed; skipping."
  fi

  # GOPATH for ubuntu user + PATH export for all shells
  mkdir -p /home/ubuntu/go
  sudo chown -R ubuntu:ubuntu /home/ubuntu/go

  # Make Go available system-wide via profile.d
  sudo bash -lc '\''cat >/etc/profile.d/go.sh <<EOF
export GOROOT=/usr/local/go
export GOPATH=/home/ubuntu/go
export PATH=\$PATH:\$GOROOT/bin:\$GOPATH/bin
EOF'\''

  # Verify (use explicit path so current shell doesn’t need re-login)
  /usr/local/go/bin/go version
'

echo ">>> Done. Enter the VM with: multipass shell $VM_NAME"
