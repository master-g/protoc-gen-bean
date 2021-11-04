-include .env

TARGETS := protoc-gen-bean

PROJECTNAME := $(shell basename "$(PWD)")

# Go
GOPROXY := https://goproxy.cn
GOBIN := $(shell pwd)/bin
GOFILES := $(wildcard *.go)
GOMODULE := github.com/master-g/protoc-gen-bean

GOLANGCILINT := $(GOBIN)/golangci-lint
GOLANGCILINT_VER := v1.43.0

# output
BIN := $(shell pwd)/bin

# Redirect error output
# STDERR := /tmp/.$(PROJECTNAME)-stderr.txt

# Make is verbose in Linux. Make it silent.
# MAKEFLAGS += --silent

## mod: Reset go mod
.PHONY: mod
mod:
	@echo "  >  Reset go mod..."
	@rm -f go.mod go.sum
	@go mod init $(GOMODULE)


## vendor: Module cleanup and vendor
.PHONY: vendor
vendor:
	@echo "  >  Module tidy and vendor..."
	@GOPROXY=$(GOPROXY) go mod tidy
	@GOPROXY=$(GOPROXY) go mod download


## lint: Lint go source files
$(GOLANGCILINT):
	@GOBIN=$(GOBIN) wget -O - -q https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s $(GOLANGCILINT_VER)


.PHONY: lint
lint: $(GOLANGCILINT)
	@echo "  >  Linting..."
	@$(GOBIN)/golangci-lint run ./...


## fmt: Formats go source files
.PHONY: fmt
fmt:
	@echo "  >  Formating..."
	@find . -type f -name '*.go' -not -path './vendor/*' -not -path './pb/*' -not -path './.idea/*' -print0 | xargs -0 goimports -w


## build: Build all executables
.PHONY: build
build: $(TARGETS)


## clean: Cleaning build cache
.PHONY: clean
clean:
	@echo "  >  Cleaning build cache..."
	@-for target in $(TARGETS); do rm -f $(BIN)/$$target; done;


$(TARGETS):
	@echo "  >  Building $@..."
	@-go build -o $(BIN)/$@ ./cmd/$@

## release: Build executable files for macOS, Windows
.PHONY: release
release:
	@echo "  >  Releasing..."
	@GOBIN=$(GOBIN) gox \
	-osarch="darwin/amd64" \
	-osarch="windows/amd64" \
	-osarch="linux/amd64" \
	-output="release/{{.OS}}_{{.Arch}}/{{.Dir}}" ./cmd/protoc-gen-bean


.PHONY: help
all: help
help: Makefile
	@echo
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@echo
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
	@echo
