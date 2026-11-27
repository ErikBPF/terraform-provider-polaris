GO ?= go

.PHONY: build fmt test docs acceptance

build:
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags '-X main.version=0.1.0-dev' -o bin/terraform-provider-polaris .

fmt:
	$(GO) fmt ./...

test:
	python3 -m unittest discover -s scripts -p 'test_*.py'
	$(GO) vet ./...
	$(GO) test -race -count=1 -timeout=5m ./...

docs:
	$(GO) generate ./...

acceptance:
	python3 scripts/acceptance.py

# Explicit context is mandatory; this target never selects the ambient context.
acceptance-minikube:
	python3 scripts/minikube.py --context "$(KUBE_CONTEXT)" -- $(GO) test ./polaris -run TestAcc -count=1 -timeout=15m
