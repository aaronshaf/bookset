.PHONY: test race vet fmt-check mod-verify staticcheck govulncheck security-check fuzz build check

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l .)"

mod-verify:
	go mod verify

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

security-check: govulncheck staticcheck

fuzz:
	go test -fuzz=Fuzz -fuzztime=10s ./internal/markdown
	go test -fuzz=Fuzz -fuzztime=10s ./internal/artifact

build:
	go build -o bin/bookset ./cmd/bookset

check: fmt-check mod-verify vet test race build
	./bin/bookset version
