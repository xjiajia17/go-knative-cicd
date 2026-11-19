APP           := go-knative-cicd
REGISTRY      := swr.cn-southwest-2.myhuaweicloud.com
# 华为云 SWR 的"组织名"，必须显式传入或在环境里设置，例如：
#   make push SWR_NAMESPACE=xjiajia
SWR_NAMESPACE ?=
VERSION       ?= v0.1.0
COMMIT        := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
IMAGE         := $(REGISTRY)/$(SWR_NAMESPACE)/$(APP)
TAG           := $(VERSION)-$(COMMIT)
GOPROXY       ?= https://goproxy.cn,direct
LDFLAGS       := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
BIN           := bin/linux/arm64/server

export GOPROXY

.PHONY: help tidy test build image push run clean

help:
	@echo "make test    - go vet + go test"
	@echo "make build   - 交叉编译 linux/arm64 静态二进制到 $(BIN)"
	@echo "make image   - build + docker build（需要 SWR_NAMESPACE）"
	@echo "make push    - image + docker push（需要 SWR_NAMESPACE，且已 docker login）"
	@echo "make run     - 本机直接跑（PORT=8080）"
	@echo "make clean   - 删除 bin/"

tidy:
	go mod tidy

test:
	go vet ./...
	go test ./...

build:
	mkdir -p $(dir $(BIN))
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='$(LDFLAGS)' -o $(BIN) ./cmd/server

image: build
	@test -n "$(SWR_NAMESPACE)" || (echo "错误：必须指定 SWR_NAMESPACE，例如 make image SWR_NAMESPACE=xjiajia"; exit 1)
	docker build --platform linux/arm64 -t $(IMAGE):$(TAG) -t $(IMAGE):latest .

push: image
	docker push $(IMAGE):$(TAG)
	docker push $(IMAGE):latest

run:
	PORT=8080 go run ./cmd/server

clean:
	rm -rf bin