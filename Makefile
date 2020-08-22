NAME:=relaymon

GO ?= go

all: $(NAME)

FORCE:

$(NAME): FORCE
	$(GO) build ./cmd/${NAME}

debug: FORCE
	$(GO) build -gcflags=all='-N -l' ./cmd/${NAME}

test: FORCE
	$(GO) test -coverprofile coverage.txt ./cmd/${NAME}
	$(GO) test -coverprofile coverage.txt  ./...

clean:
	@rm -f ./${NAME}

#prep:
#	GO111MODULE=on go get -u github.com/mailru/easyjson/...@v0.7.1
#	GO111MODULE=on go get -u github.com/go-bindata/go-bindata/...@v3.1.2+incompatible

#gen:
#	easyjson -all pkg/aggstat/aggstat.go
#	easyjson -all pkg/aggstatcmp/aggstatcmp.go
#	easyjson -all pkg/datatables/datatables.go
#	go-bindata -o cmd/jmeterstat/bindata.go web web/template

lint:
	golangci-lint run
