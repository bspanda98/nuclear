
export GO111MODULE=off
export GOFLAGS=

go get -u github.com/fjl/gencodec
go get -u golang.org/x/tools/cmd/stringer
go get -u github.com/go-bindata/go-bindata/...

export GO111MODULE=on
export GOFLAGS=-mod=vendor

go generate nuclear/core/nuclear/core/types
go generate nuclear/core/nuclear/core/vm
go generate nuclear/core/nuclear/core
go generate nuclear/core/nuclear/eth/tracers/internal/tracers/
go generate nuclear/core/nuclear/eth/
go generate nuclear/core/nuclear/internal/jsre/deps/
go generate nuclear/core/nuclear/p2p/discv5
go generate nuclear/core/nuclear/signer/rules/deps
go generate nuclear/core/nuclear/whisper/whisperv6/

