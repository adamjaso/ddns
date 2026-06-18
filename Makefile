SHELL := /bin/bash

DDNS := ddns
DDNS_USER := $(DDNS)
DDNS_GROUP ?= $(DDNS)
SERVICE := $(DDNS).service
INSTALL_PREFIX ?=
CONF := $(INSTALL_PREFIX)/etc/ddns/env
DDNS_PATH := $(INSTALL_PREFIX)/usr/local/bin/$(DDNS)
SYSTEMCTL ?= systemctl

$(DDNS) build:
	go build -o $(DDNS)

clean:
	rm $(DDNS) 2>/dev/null ||:

install: $(DDNS)
	@echo ">>> Creating $(DDNS_USER) user..."
	getent passwd $(DDNS_USER) | grep -q $(DDNS_USER) || useradd --no-create-home --system --shell /sbin/nologin $(DDNS_USER)
	@echo ">>> Generating ddns config..."
	mkdir -p $(INSTALL_PREFIX)/etc/ddns
	mkdir -p $(INSTALL_PREFIX)/usr/local/bin
	mkdir -p $(INSTALL_PREFIX)/etc/systemd/system
	cp -an ddns.env $(CONF)
	cp -a $(DDNS) $(DDNS_PATH)
	@echo ">>> Suggested password: $$(uuidgen)"
	@$(DDNS_PATH) -genhash >> $(CONF)
	@read -p '>>> Enter your dynamically managed DNS domains: ' DOMAINS ; echo "DOMAINS=$${DOMAINS}" >> $(CONF)
	echo '{}' > $(INSTALL_PREFIX)/etc/ddns/cache.json
	chown -R $(DDNS_USER):$(DDNS_GROUP) $(INSTALL_PREFIX)/etc/ddns
	cp -a $(SERVICE) $(INSTALL_PREFIX)/etc/systemd/system/$(SERVICE)
	$(SYSTEMCTL) daemon-reload
	$(SYSTEMCTL) enable $(SERVICE)
	$(SYSTEMCTL) start $(SERVICE)
	$(SYSTEMCTL) status $(SERVICE)

reload:
	$(SYSTEMCTL) daemon-reload
	$(SYSTEMCTL) restart $(SERVICE)

logs:
	journalctl -u $(SERVICE) -n 20 -f
	
.PHONY: build clean install reload logs
