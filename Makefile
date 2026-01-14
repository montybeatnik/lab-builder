setup_git:
	git config --global user.email "christopher.t.hern@gmail.com"
	git config --global user.name "Christopher Hern"

VM_NAME ?= lab-builder
VM_LAB_DIR ?= /home/ubuntu/lab

vm_setup:
	AUTO_DEPLOY=0 ./setup-multipass.sh

vm_shell:
	multipass shell $(VM_NAME)

vm_server:
	multipass exec $(VM_NAME) -- bash -lc 'cd $(VM_LAB_DIR)/src && /usr/local/go/bin/go run .'

vm_server_bg:
	multipass exec $(VM_NAME) -- bash -lc 'cd $(VM_LAB_DIR)/src || exit 1; : > /tmp/arista-lab-server.log; /usr/local/go/bin/go build -o /tmp/arista-lab-server . && pkill -f "/tmp/arista-lab-server" || true; setsid /tmp/arista-lab-server > /tmp/arista-lab-server.log 2>&1 < /dev/null & echo $$! > /tmp/arista-lab-server.pid; sleep 1; exit 0'

vm_server_logs:
	multipass exec $(VM_NAME) -- bash -lc 'test -f /tmp/arista-lab-server.log && tail -n 200 /tmp/arista-lab-server.log || echo "no log file yet"'

vm_server_status:
	multipass exec $(VM_NAME) -- bash -lc 'ss -lntp | grep 8080 || true'

vm_server_stop:
	multipass exec $(VM_NAME) -- bash -lc 'if test -f /tmp/arista-lab-server.pid; then kill $$(cat /tmp/arista-lab-server.pid) || true; else pkill -f "go run ." || true; fi'

vm_server_build:
	multipass exec $(VM_NAME) -- bash -lc 'cd $(VM_LAB_DIR)/src && /usr/local/go/bin/go mod tidy && /usr/local/go/bin/go build -o /tmp/arista-lab-server .'

vm_server_run:
	multipass exec $(VM_NAME) -- bash -lc '/tmp/arista-lab-server'

vm_server_install:
	multipass exec $(VM_NAME) -- bash -lc 'mkdir -p /home/ubuntu/.config/systemd/user && printf "%s\n" "[Unit]" "Description=Arista Lab Go Server" "After=network.target" "" "[Service]" "WorkingDirectory=$(VM_LAB_DIR)/src" "ExecStart=/tmp/arista-lab-server" "Restart=on-failure" "Environment=CLAB_LABDIR_BASE=/home/ubuntu/.clab-runs" "" "[Install]" "WantedBy=default.target" > /home/ubuntu/.config/systemd/user/arista-lab-server.service'

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

vm_rebuild:
	$(MAKE) vm_server_build
	$(MAKE) vm_server_stop_service
	$(MAKE) vm_server_start

vm_ui_logs:
	multipass exec lab-builder -- bash -lc 'journalctl --user -u arista-lab-server.service -f'

vm_ui:
	@multipass exec $(VM_NAME) -- bash -lc 'if command -v ufw >/dev/null 2>&1; then sudo ufw allow 8080/tcp || true; fi'
	@ip=$$(multipass info $(VM_NAME) | awk "/IPv4/ {print \$$2; exit}"); \
	if [ -z "$$ip" ]; then \
		echo "Could not determine VM IPv4 address"; exit 1; \
	fi; \
	echo "UI available at: http://$$ip:8080"

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
