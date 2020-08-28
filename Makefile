NAME:=relaymon
INTEGRATION:=./relaymontest.py

VERSION := $(shell git describe --always --tags)

GO ?= go

all: $(NAME)

FORCE:

$(NAME): FORCE
	$(GO) build -ldflags "-X main.version=${VERSION}" ./cmd/${NAME}

debug: FORCE
	$(GO) build -gcflags=all='-N -l' -ldflags "-X main.version=${VERSION}" ./cmd/${NAME}

test: FORCE
	$(GO) test -coverprofile coverage.txt ./cmd/${NAME}
	$(GO) test -coverprofile coverage.txt  ./...

integration: FORCE
	${INTEGRATION}

clean:
	@rm -f ./${NAME}

lint:
	golangci-lint run
