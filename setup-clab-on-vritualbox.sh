#!/usr/bin/env bash
set -Eeuo pipefail
trap 'echo "ERROR on line $LINENO. Exiting." >&2' ERR

### helper funcs
install_ceos_image() {
  echo ">>> Installing cEOS-lab image into Docker…"
  [[ -f "$CEOS_TARBALL" ]] || { echo "ERROR: cEOS tarball not found at $CEOS_TARBALL"; exit 1; }

  if sudo docker image inspect "$CEOS_TAG" >/dev/null 2>&1; then
    echo " - Docker image '$CEOS_TAG' already present; skipping import."
    return 0
  fi

  # Choose the right loader: Arista ships cEOS as a tarball for `docker import`
  case "$CEOS_TARBALL" in
    *.tar|*.tar.gz|*.tgz|*.tar.xz)
      echo " - Importing $(basename "$CEOS_TARBALL") as $CEOS_TAG"
      sudo docker import "$CEOS_TARBALL" "$CEOS_TAG"
      ;;
    *.docker|*.tar?.lz4)
      # If you ever saved it with `docker save`, use `docker load` instead.
      echo " - Loading docker-archive $CEOS_TARBALL"
      sudo docker load -i "$CEOS_TARBALL"
      ;;
    *)
      echo "ERROR: Unknown cEOS archive type: $CEOS_TARBALL"
      exit 1
      ;;
  esac

  echo " - Verifying image:"
  sudo docker image inspect "$CEOS_TAG" --format '  RepoTags: {{.RepoTags}} | Arch: {{.Architecture}}'
}

### ───────────────────────────
### EDIT THESE FOR YOUR SETUP
### ───────────────────────────
# If you configured a VirtualBox Shared Folder, set its NAME here (VirtualBox UI → Settings → Shared Folders)
VB_SHARE_NAME="${VB_SHARE_NAME:-lab}"      # name seen in VBox, e.g., "lab"
LAB_MOUNT="${LAB_MOUNT:-$HOME/lab}"        # where to mount inside the VM

# If you prefer git instead of a shared folder, set REPO_URL (used only if share can’t be mounted)
REPO_URL="${REPO_URL:-https://github.com/montybeatnik/arista-lab.git}"
REPO_BRANCH="${REPO_BRANCH:-main}"

# Optional: cEOS tarball path *inside this VM* (leave empty to skip)
CEOS_TARBALL="${CEOS_TARBALL:-}"
CEOS_TAG="${CEOS_TAG:-ceosimage:4.34.2.1f}"

# Lab file path relative to repo root
LAB_FILE="${LAB_FILE:-lab.clab.yml}"

# Auto deploy after setup? (1=yes, 0=no)
AUTO_DEPLOY="${AUTO_DEPLOY:-1}"

### ───────────────────────────
### helpers
### ───────────────────────────
err()  { echo "ERROR: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || err "Missing dependency '$1'."; }

say()  { printf "\n>>> %s\n" "$*"; }

### ───────────────────────────
### base packages
### ───────────────────────────
say "Installing base packages..."
sudo apt-get update -y
sudo apt-get install -y ca-certificates curl gnupg lsb-release jq git iproute2 arping iperf3

### ───────────────────────────
### Docker + Containerlab
### (official quick-setup paths for fast provisioning)
### ───────────────────────────
if ! command -v docker >/dev/null 2>&1; then
  say "Installing Docker via convenience script..."
  curl -fsSL https://get.docker.com | sh   # quick provisioning path
  sudo usermod -aG docker "$USER" || true
fi

if ! command -v containerlab >/dev/null 2>&1; then
  say "Installing Containerlab..."
  # all-in-one setup installs docker prereqs if needed on Debian/RHEL-like
  curl -sL https://containerlab.dev/setup | sudo -E bash -s "all"
fi

# make sure current shell can use docker group if newly added
if ! groups | grep -q '\bdocker\b'; then
  say "You were added to 'docker' group. Re-login or run: newgrp docker"
fi

### Install cEOS into docker
# point CEOS_TARBALL at the actual file then install
CEOS_TARBALL="${CEOS_TARBALL:-$HOME/images/cEOSarm-lab-4.34.2.1F.tar.xz}"
install_ceos_image

### ───────────────────────────
### Repo: try VirtualBox shared folder first, else git clone
### ───────────────────────────
mkdir -p "$LAB_MOUNT"
if [ -e /sbin/mount.vboxsf ] || [ -e /usr/sbin/mount.vboxsf ]; then
  # Guest Additions likely present; try to mount named share
  say "Attempting to mount VirtualBox shared folder '$VB_SHARE_NAME' at '$LAB_MOUNT'..."
  sudo apt-get install -y virtualbox-guest-utils || true
  sudo usermod -aG vboxsf "$USER" || true   # access shared folder
  # try mount; if it fails, we will fallback to git
  if sudo mount -t vboxsf -o uid="$UID",gid="$(id -g)" "$VB_SHARE_NAME" "$LAB_MOUNT" 2>/dev/null; then
    say "Mounted VirtualBox share '$VB_SHARE_NAME' at '$LAB_MOUNT'."
  else
    say "Shared folder mount failed. Falling back to git clone."
    rm -rf "$LAB_MOUNT/.git" 2>/dev/null || true
    if [ ! -d "$LAB_MOUNT/.git" ]; then
      git clone --branch "$REPO_BRANCH" "$REPO_URL" "$LAB_MOUNT"
    fi
  fi
else
  say "VirtualBox Guest Additions/driver not detected. Cloning repo instead."
  if [ ! -d "$LAB_MOUNT/.git" ]; then
    git clone --branch "$REPO_BRANCH" "$REPO_URL" "$LAB_MOUNT"
  fi
fi

[ -f "$LAB_MOUNT/$LAB_FILE" ] || err "Lab file not found: $LAB_MOUNT/$LAB_FILE"

### ───────────────────────────
### Import cEOS image (optional)
### ───────────────────────────
if [ -n "$CEOS_TARBALL" ]; then
  say "Importing cEOS image from '$CEOS_TARBALL' as '$CEOS_TAG'..."
  if ! sudo docker image inspect "$CEOS_TAG" >/dev/null 2>&1; then
    sudo docker import "$CEOS_TARBALL" "$CEOS_TAG"
  else
    say "Docker image '$CEOS_TAG' already present; skipping import."
  fi
  # Arch sanity (best effort)
  IMG_ARCH="$(sudo docker inspect "$CEOS_TAG" --format '{{.Architecture}}' 2>/dev/null || true)"
  HOST_ARCH="$(uname -m)"
  say "Image arch: ${IMG_ARCH:-unknown} | Host arch: $HOST_ARCH"
fi

### ───────────────────────────
### Keep clab run-artifacts out of your repo mount
### ───────────────────────────
mkdir -p "$HOME/.clab-runs"
if ! grep -q "CLAB_LABDIR_BASE" "$HOME/.bashrc"; then
  echo 'export CLAB_LABDIR_BASE=$HOME/.clab-runs' >> "$HOME/.bashrc"
fi
export CLAB_LABDIR_BASE="$HOME/.clab-runs"

### ───────────────────────────
### Show versions
### ───────────────────────────
say "Versions:"
docker --version || true
containerlab version || true
echo
say "Lab root at: $LAB_MOUNT"
ls -la "$LAB_MOUNT" | sed -n '1,80p'

### ───────────────────────────
### ensure gNMIc image matches host arch
### ───────────────────────────
say "Pre-pulling gNMIc image for host architecture..."
GNMIC_IMAGE="ghcr.io/openconfig/gnmic:latest"
ARCH="$(uname -m)"
if [[ "$ARCH" == "arm64" || "$ARCH" == "aarch64" ]]; then
  sudo docker pull --platform=linux/arm64 "$GNMIC_IMAGE"
else
  sudo docker pull "$GNMIC_IMAGE"
fi

### ───────────────────────────
### Deploy (optional)
### ───────────────────────────
if [ "${AUTO_DEPLOY}" = "1" ]; then
  say "Deploying lab: $LAB_MOUNT/$LAB_FILE"
  pushd "$LAB_MOUNT" >/dev/null
  sudo -E containerlab destroy -t "$LAB_FILE" || true
  sudo -E containerlab deploy  -t "$LAB_FILE" --reconfigure
  echo
  say "Lab status:"
  containerlab inspect -t "$LAB_FILE" | sed -n '1,120p' || true
  popd >/dev/null
else
  cat <<EOF

>>> Skipping auto-deploy (AUTO_DEPLOY=0).
To deploy manually:
  cd "$LAB_MOUNT"
  export CLAB_LABDIR_BASE=\$HOME/.clab-runs
  sudo -E containerlab deploy -t "$LAB_FILE" --reconfigure

EOF
fi

say "Done."
