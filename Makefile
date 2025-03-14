.PHONY: clean test gen deb
.ONESHELL:
.SHELLFLAGS += -x -e
PWD = $(shell pwd)
UID = $(shell id -u)
GID = $(shell id -g)
VERSION = "0.1.0"
LD_FLAGS = -X goauthentik.io/cli/pkg/storage.Version=${VERSION}
GO_FLAGS = -ldflags "${LD_FLAGS}" -v
MODULE := pam_authentik
PAM_OUTPUT := ./bin/pam/
LOCAL_BUILD_ARCH := linux/amd64

all: clean gen bin/ak bin/ak-agent bin/pam/$(MODULE).so

bin/ak:
	$(eval LD_FLAGS := -X goauthentik.io/cli/pkg/storage.Version=${VERSION} -X goauthentik.io/cli/pkg/storage.BuildHash=dev-$(shell git rev-parse HEAD))
	go build \
		-ldflags "${LD_FLAGS} -X goauthentik.io/cli/pkg/storage.BuildHash=${GIT_BUILD_HASH}" \
		-v -a -o ${PWD}/bin/ak \
		${PWD}/cmd/cli/main

bin/ak-agent:
	$(eval LD_FLAGS := -X goauthentik.io/cli/pkg/storage.Version=${VERSION} -X goauthentik.io/cli/pkg/storage.BuildHash=dev-$(shell git rev-parse HEAD))
	go build \
		-ldflags "${LD_FLAGS} -X goauthentik.io/cli/pkg/storage.BuildHash=${GIT_BUILD_HASH}" \
		-v -a -o ${PWD}/bin/ak-agent \
		${PWD}/cmd/agent
	cp -R "${PWD}/package/macos/authentik Agent.app" ${PWD}/bin/
	mkdir -p "${PWD}/bin/authentik Agent.app/Contents/MacOS"
	cp ${PWD}/bin/ak-agent "${PWD}/bin/authentik Agent.app/Contents/MacOS/"

clean:
	rm -rf ${PWD}/bin/*
	rm -rf ${PWD}/*.h

bin/pam/$(MODULE).so: .
	$(eval LD_FLAGS := -X goauthentik.io/cli/pkg/storage.Version=${VERSION} -X goauthentik.io/cli/pkg/storage.BuildHash=dev-$(shell git rev-parse HEAD))
	go build \
		-ldflags "${LD_FLAGS} -X goauthentik.io/cli/pkg/storage.BuildHash=${GIT_BUILD_HASH}" \
		-v -buildmode=c-shared -o bin/pam/$(MODULE).so ${PWD}/cmd/pam/

bin/pam/deb: bin/pam/$(MODULE).so
	mkdir -p $(PAM_OUTPUT)
	$(shell go env GOPATH)/bin/nfpm package -p deb -t $(PAM_OUTPUT) -f ${PWD}/cmd/pam/nfpm.yaml

pam-docker: clean gen
	cd ${PWD}/hack/pam/local_build && docker build \
		--platform $(LOCAL_BUILD_ARCH) \
		--tag pam_authentik:local_build \
		.
	docker run \
		-it \
		--platform $(LOCAL_BUILD_ARCH) \
		--rm \
		-v ${PWD}:/data \
		-v pam_authentik-go-cache:/root/go/pkg \
		-v pam_authentik-go-build-cache:/root/.cache \
		pam_authentik:local_build \
		make bin/pam/deb

gen:
	go generate ./...
