.PHONY: build test clean lint

build:
	go build -o bin/kubectl-fields ./cmd/kubectl-fields

test:
	go test ./... -v

clean:
	rm -rf bin/

lint:
	go vet ./...
