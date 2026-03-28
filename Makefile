.DEFAULT_GOAL := vm_up

.PHONY: \
	setup_git \
	vm_up \
	vm_ensure \
	vm_mount_repo \
	vm_setup \
	vm_deploy \
	vm_shell \
	vm_server \
	vm_server_bg \
	vm_server_logs \
	vm_server_status \
	vm_server_stop \
	vm_server_test \
	vm_server_build \
	vm_server_run \
	vm_server_install \
	vm_server_start \
	vm_server_stop_service \
	vm_rebuild \
	vm_server_status_service \
	vm_ui_logs \
	vm_ui \
	vm_monitoring \
	lab_status \
	test_data_plane \
	fix_acl_perm_issue

setup_git:
	git config --global user.email "christopher.t.hern@gmail.com"
	git config --global user.name "Christopher Hern"

VM_NAME ?= lab-builder
VM_LAB_DIR ?= /home/ubuntu/lab
HOST_REPO_DIR ?= $(CURDIR)

vm_up: vm_ensure vm_server_test vm_server_build vm_server_install vm_server_start vm_server_status_service vm_ui
	@echo "VM and server are ready."

vm_ensure:
	@set -e; \
	if ! command -v multipass >/dev/null 2>&1; then \
		echo "ERROR: Multipass is not installed."; \
		exit 1; \
	fi; \
	if ! multipass info $(VM_NAME) >/dev/null 2>&1; then \
		echo "VM '$(VM_NAME)' not found. Running vm_setup..."; \
		$(MAKE) vm_setup; \
	fi; \
	vm_state=$$(multipass info $(VM_NAME) | awk -F: '/State/ {gsub(/^[ \t]+/,"",$$2); print $$2; exit}'); \
	if [ "$$vm_state" != "Running" ]; then \
		echo "Starting VM '$(VM_NAME)'..."; \
		multipass start $(VM_NAME); \
	fi

lab_status:
	@set -e; \
	echo "Lab status check ($(VM_NAME))"; \
	if ! command -v multipass >/dev/null 2>&1; then \
		echo "ERROR: Multipass not installed. Install Multipass first."; \
		exit 0; \
	fi; \
	if ! multipass info $(VM_NAME) >/dev/null 2>&1; then \
		echo "ERROR: VM '$(VM_NAME)' not found."; \
		echo "  Next: run 'make vm_setup' to create the VM."; \
		exit 0; \
	fi; \
	vm_state=$$(multipass info $(VM_NAME) | awk -F: '/State/ {gsub(/^[ \t]+/,"",$$2); print $$2; exit}'); \
	echo "OK: VM exists (state: $$vm_state)"; \
	if [ "$$vm_state" != "Running" ]; then \
		echo "  Next: run 'multipass start $(VM_NAME)'"; \
		exit 0; \
	fi; \
	multipass exec $(VM_NAME) -- bash -lc ' \
		set -e; \
		echo "OK: VM reachable"; \
		if ! command -v docker >/dev/null 2>&1; then \
			echo "ERROR: Docker not installed in VM"; \
			echo "  Next: run ./setup-multipass.sh (or re-run make vm_setup)"; \
			exit 0; \
		fi; \
		echo "OK: Docker installed"; \
		if ! command -v containerlab >/dev/null 2>&1; then \
			echo "ERROR: Containerlab not installed in VM"; \
			echo "  Next: run ./setup-multipass.sh (or re-run make vm_setup)"; \
			exit 0; \
		fi; \
		echo "OK: Containerlab installed"; \
		if [ ! -f $(VM_LAB_DIR)/lab.clab.yml ]; then \
			echo "ERROR: Lab file not found at $(VM_LAB_DIR)/lab.clab.yml"; \
			echo "  Next: ensure repo is mounted in the VM at $(VM_LAB_DIR)"; \
			exit 0; \
		fi; \
		export CLAB_LABDIR_BASE=$$HOME/.clab-runs; \
		if sudo -E containerlab inspect -t $(VM_LAB_DIR)/lab.clab.yml >/dev/null 2>&1; then \
			echo "OK: Lab is deployed"; \
		else \
			echo "ERROR: Lab not deployed"; \
			echo "  Next: run 'make vm_deploy'"; \
		fi; \
	'; \
	echo "Done."

vm_setup:
	AUTO_DEPLOY=0 ./setup-multipass.sh

vm_mount_repo: vm_ensure
	@set -e; \
	if multipass exec $(VM_NAME) -- bash -lc 'test -f "$(VM_LAB_DIR)/src/go.mod" || test -f "$(VM_LAB_DIR)/go.mod"' >/dev/null 2>&1; then \
		echo "Repo mount OK at $(VM_LAB_DIR)"; \
	else \
		echo "Repo mount missing/broken at $(VM_LAB_DIR); repairing mount"; \
		multipass umount $(VM_NAME):$(VM_LAB_DIR) >/dev/null 2>&1 || true; \
		multipass exec $(VM_NAME) -- bash -lc 'mkdir -p "$(VM_LAB_DIR)"' >/dev/null 2>&1 || true; \
		echo "Mounting $(HOST_REPO_DIR) -> $(VM_LAB_DIR)"; \
		multipass mount $(HOST_REPO_DIR) $(VM_NAME):$(VM_LAB_DIR); \
		if multipass exec $(VM_NAME) -- bash -lc 'test -f "$(VM_LAB_DIR)/src/go.mod" || test -f "$(VM_LAB_DIR)/go.mod"' >/dev/null 2>&1; then \
			echo "Repo mount repaired at $(VM_LAB_DIR)"; \
		else \
			echo "ERROR: mount completed but Go app dir still missing under $(VM_LAB_DIR)"; \
			echo "Run: multipass info $(VM_NAME)"; \
			exit 1; \
		fi; \
	fi

vm_deploy: vm_mount_repo
	multipass exec $(VM_NAME) -- bash -lc 'set -euo pipefail; cd $(VM_LAB_DIR); arch=$$(uname -m); if [[ "$$arch" == "arm64" || "$$arch" == "aarch64" ]]; then sudo docker pull --platform=linux/arm64 ghcr.io/openconfig/gnmic:latest; else sudo docker pull ghcr.io/openconfig/gnmic:latest; fi; sudo docker pull quay.io/frrouting/frr:9.1.3 || sudo docker pull quay.io/frrouting/frr:latest; export CLAB_LABDIR_BASE=$$HOME/.clab-runs; sudo -E containerlab deploy -t lab.clab.yml --reconfigure'

vm_shell:
	multipass shell $(VM_NAME)

vm_server: vm_mount_repo
	multipass exec $(VM_NAME) -- bash -lc 'set -euo pipefail; if [ -f "$(VM_LAB_DIR)/src/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)/src"; elif [ -f "$(VM_LAB_DIR)/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)"; else echo "ERROR: Go app dir not found under $(VM_LAB_DIR)"; exit 1; fi; cd "$$APP_DIR"; /usr/local/go/bin/go run .'

vm_server_bg: vm_mount_repo
	multipass exec $(VM_NAME) -- bash -lc 'set -euo pipefail; if [ -f "$(VM_LAB_DIR)/src/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)/src"; elif [ -f "$(VM_LAB_DIR)/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)"; else echo "ERROR: Go app dir not found under $(VM_LAB_DIR)"; exit 1; fi; cd "$$APP_DIR"; : > /tmp/arista-lab-server.log; /usr/local/go/bin/go build -o /tmp/arista-lab-server . && pkill -f "/tmp/arista-lab-server" || true; setsid /tmp/arista-lab-server > /tmp/arista-lab-server.log 2>&1 < /dev/null & echo $$! > /tmp/arista-lab-server.pid; sleep 1; exit 0'

vm_server_logs:
	multipass exec $(VM_NAME) -- bash -lc 'test -f /tmp/arista-lab-server.log && tail -n 200 /tmp/arista-lab-server.log || echo "no log file yet"'

vm_server_status:
	multipass exec $(VM_NAME) -- bash -lc 'ss -lntp | grep 8080 || true'

vm_server_stop:
	multipass exec $(VM_NAME) -- bash -lc 'if test -f /tmp/arista-lab-server.pid; then kill $$(cat /tmp/arista-lab-server.pid) || true; else pkill -f "go run ." || true; fi'

vm_server_test: vm_mount_repo
	multipass exec $(VM_NAME) -- bash -lc 'set -euo pipefail; if [ -f "$(VM_LAB_DIR)/src/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)/src"; elif [ -f "$(VM_LAB_DIR)/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)"; else echo "ERROR: Go app dir not found under $(VM_LAB_DIR)"; exit 1; fi; cd "$$APP_DIR"; /usr/local/go/bin/go test .'

vm_server_build: vm_mount_repo
	multipass exec $(VM_NAME) -- bash -lc 'set -euo pipefail; if [ -f "$(VM_LAB_DIR)/src/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)/src"; elif [ -f "$(VM_LAB_DIR)/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)"; else echo "ERROR: Go app dir not found under $(VM_LAB_DIR)"; exit 1; fi; cd "$$APP_DIR"; /usr/local/go/bin/go mod tidy && /usr/local/go/bin/go build -o /tmp/arista-lab-server .'

vm_server_run:
	multipass exec $(VM_NAME) -- bash -lc '/tmp/arista-lab-server'

vm_server_install: vm_mount_repo
	multipass exec $(VM_NAME) -- bash -lc 'set -euo pipefail; if [ -f "$(VM_LAB_DIR)/src/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)/src"; elif [ -f "$(VM_LAB_DIR)/go.mod" ]; then APP_DIR="$(VM_LAB_DIR)"; else echo "ERROR: Go app dir not found under $(VM_LAB_DIR)"; exit 1; fi; mkdir -p /home/ubuntu/.config/systemd/user && printf "%s\n" "[Unit]" "Description=Arista Lab Go Server" "After=network.target" "" "[Service]" "WorkingDirectory=$$APP_DIR" "ExecStart=/tmp/arista-lab-server" "Restart=on-failure" "Environment=CLAB_LABDIR_BASE=/home/ubuntu/.clab-runs" "" "[Install]" "WantedBy=default.target" > /home/ubuntu/.config/systemd/user/arista-lab-server.service'

vm_server_start:
	multipass exec $(VM_NAME) -- bash -lc 'systemctl --user daemon-reload && systemctl --user enable --now arista-lab-server.service'

vm_server_stop_service:
	multipass exec $(VM_NAME) -- bash -lc 'systemctl --user stop arista-lab-server.service || true'

vm_rebuild:
	$(MAKE) vm_server_build
	$(MAKE) vm_server_stop_service
	$(MAKE) vm_server_start

vm_server_status_service:
	multipass exec $(VM_NAME) -- bash -lc 'systemctl --user status arista-lab-server.service --no-pager'

vm_ui_logs:
	multipass exec lab-builder -- bash -lc 'journalctl --user -u arista-lab-server.service -f'

vm_ui:
	@multipass exec $(VM_NAME) -- bash -lc 'if command -v ufw >/dev/null 2>&1; then sudo ufw allow 8080/tcp || true; fi'
	@ip=$$(multipass info $(VM_NAME) | awk "/IPv4/ {print \$$2; exit}"); \
	if [ -z "$$ip" ]; then \
		echo "Could not determine VM IPv4 address"; exit 1; \
	fi; \
	echo "UI available at: http://$$ip:8080"

vm_monitoring:
	@multipass exec $(VM_NAME) -- bash -lc 'if command -v ufw >/dev/null 2>&1; then sudo ufw allow 3000/tcp || true; sudo ufw allow 9090/tcp || true; fi'
	@ip=$$(multipass info $(VM_NAME) | awk "/IPv4/ {print \$$2; exit}"); \
	if [ -z "$$ip" ]; then \
		echo "Could not determine VM IPv4 address"; exit 1; \
	fi; \
	echo "Grafana available at: http://$$ip:3000"; \
	echo "Prometheus available at: http://$$ip:9090"

test_data_plane:
	sudo docker exec -it clab-evpn-rdma-fabric-gpu1 sh -lc 'ping -c3 10.10.10.104'

fix_acl_perm_issue:
	# 1) Pick a local (ext4) path for clab’s runtime
	mkdir -p ~/.clab-runs
	
	# 2) Ensure the env var is set AND preserved for sudo
	export CLAB_LABDIR_BASE=$HOME/.clab-runs
	sudo -E env | grep CLAB_LABDIR_BASE || echo "env not preserved!"
	
	# 3) Clean any half-created labdir on the mounted repo (optional but tidy)
	rm -rf ~/lab/clab-evpn-rdma-fabric
	
	# 4) Redeploy (note the -E to keep the env var)
	sudo -E containerlab destroy  -t ~/lab/lab.clab.yml || true
	sudo -E containerlab deploy   -t ~/lab/lab.clab.yml --reconfigure
