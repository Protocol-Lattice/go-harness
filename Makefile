.PHONY: build
build: build-harness build-providers

.PHONY: build-harness
build-harness:
	mkdir -p ./bin
	go build -o ./bin/go-harness ./cmd/go-harness

.PHONY: build-providers
build-providers: build-filesystem build-shell build-git

.PHONY: build-filesystem
build-filesystem:
	mkdir -p ./bin/filesystem
	go build -o ./bin/filesystem/filesystem ./cmd/filesystem

.PHONY: build-shell
build-shell:
	mkdir -p ./bin/shell
	go build -o ./bin/shell/shell ./cmd/shell

.PHONY: build-git
build-git:
	mkdir -p ./bin/git
	go build -o ./bin/git/git ./cmd/git

.PHONY: install
install:
	go install ./cmd/go-harness

.PHONY: doctor
doctor: build
	./bin/go-harness doctor

.PHONY: test
test:
	go test ./...

.PHONY: tidy
tidy:
	go mod tidy
