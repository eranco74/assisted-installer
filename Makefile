INSTALLER := $(or ${INSTALLER},quay.io/ocpmetal/assisted-installer:stable)
GIT_REVISION := $(shell git rev-parse HEAD)
CONTROLLER :=  $(or ${CONTROLLER}, quay.io/ocpmetal/assisted-installer-controller:stable)
CONTAINER_RUNTIME := $(shell command -v podman 2> /dev/null || echo docker)
UID = $(shell id -u)

all: deps generate-from-swagger generate image image_controller unit-test

lint:
	golangci-lint run -v

format:
	goimports -w -l src/ || /bin/true

deps:
	GOSUMDB=off go mod download

generate:
	go generate $(shell go list ./...)
	$(MAKE) format

generate-from-swagger: clean
	mkdir -p ./generated/bm-inventory
	cp $(shell go list -m -f={{.Dir}} github.com/filanov/bm-inventory)/swagger.yaml ./generated/bm-inventory/swagger.yaml
	chown $(UID):$(UID) ./generated/bm-inventory/swagger.yaml
	$(CONTAINER_RUNTIME) run -u $(UID):$(UID) -v $(PWD):$(PWD):rw,Z -v /etc/passwd:/etc/passwd -w $(PWD) \
		quay.io/goswagger/swagger:v0.24.0 generate client --template=stratoscale -f ./generated/bm-inventory/swagger.yaml \
		--template-dir=/templates/contrib -t $(PWD)/generated/bm-inventory

unit-test:
	go test -v $(shell go list ./...) -cover -ginkgo.focus=${FOCUS} -ginkgo.v

ut:
	go test -v -coverprofile=coverage.out ./... && go tool cover -html=coverage.out && rm coverage.out

build/installer: lint format
	mkdir -p build
	CGO_ENABLED=0 go build -o build/installer src/main/main.go

build/controller: lint format
	mkdir -p build
	CGO_ENABLED=0 go build -o build/assisted-installer-controller src/main/assisted-installer-controller/assisted_installer_main.go

image: build/installer
	GIT_REVISION=${GIT_REVISION} $(CONTAINER_RUNTIME) build --build-arg GIT_REVISION -f Dockerfile.assisted-installer . -t $(INSTALLER)

push: image
	$(CONTAINER_RUNTIME) push $(INSTALLER)

image_controller: build/controller
	GIT_REVISION=${GIT_REVISION} $(CONTAINER_RUNTIME) build --build-arg GIT_REVISION -f Dockerfile.assisted-installer-controller . -t $(CONTROLLER)

push_controller: image_controller
	$(CONTAINER_RUNTIME) push $(CONTROLLER)

clean:
	-rm -rf build generated
