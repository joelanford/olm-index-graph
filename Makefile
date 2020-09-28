CGO_ENABLED:=1

.PHONY: all
all: build

.PHONY: build
build:
	go build -o bin/olm-index-graph ./

.PHONY: install
install:
	go install

.PHONY: clean
clean:
	rm -r bin

