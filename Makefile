.PHONY: build
build:
	mkdir -p ./bin
	go build -o ./bin/go-harness ./cmd/go-harness

.PHONY: build-filesystem
build-filesystem:
	mkdir -p ./bin
	go build -o ./bin/filesystem ./cmd/filesystem

.PHONY: install
install:
	go install ./cmd/go-harness

.PHONY: doctor
doctor: build build-filesystem
	./bin/go-harness doctor

.PHONY: test
test:
	go test ./...

.PHONY: tidy
tidy:
	go mod tidy