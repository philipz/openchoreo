# All the make targets are implemented in the make/*.mk files.
# To see all the available targets, run `make help`.

PROJECT_DIR := $(realpath $(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

#-----------------------------------------------------------------------------
# Makefile includes
#-----------------------------------------------------------------------------
include make/common.mk
include make/tools.mk
include make/golang.mk
include make/lint.mk
include make/docker.mk
include make/kube.mk
include make/helm.mk
include make/kind.mk

OBSERVABILITY_NAMESPACE ?= openchoreo-observability-plane

.PHONY: deploy-observability
deploy-observability: ## Deploy OTLP collector DaemonSet and config to the current cluster
	@$(call log_info, Creating namespace '$(OBSERVABILITY_NAMESPACE)' if needed...)
	@kubectl create namespace $(OBSERVABILITY_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@$(call log_info, Applying OpenTelemetry collector manifests...)
	kubectl apply -k config/observability/collectors/otel/base
