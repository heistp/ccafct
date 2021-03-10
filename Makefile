.PHONY: build
build:
	CGO_ENABLED=0 go build -o fct ./cmd/fct
	CGO_ENABLED=0 go build -o ccafct ./cmd/ccafct

.PHONY: install
install:
	go install fct
	go install ccafct

.PHONY: clean
clean:
	rm -f fct ccafct
