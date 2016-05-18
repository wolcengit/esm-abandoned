SHELL=/bin/bash
CWD=$(shell pwd)
OLDGOPATH=${GOPATH}
NEWGOPATH:=${CWD}:${OLDGOPATH}
export GOPATH=$(NEWGOPATH)


build: clean config
	go build  -o bin/esmove

tar: build
	tar cfz bin/esmove.tar.gz bin/esmove

cross-build: clean config
	go test
	GOOS=windows GOARCH=amd64 go build -o bin/windows64/esmove.exe
	GOOS=darwin  GOARCH=amd64 go build -o bin/darwin64/esmove
	GOOS=linux  GOARCH=amd64 go build -o bin/linux64/esmove


all: clean config cross-build



format:
	gofmt -s -w -tabs=false -tabwidth=4 main.go

clean:
	rm -rif bin
	mkdir bin
	mkdir bin/windows64
	mkdir bin/linux64
	mkdir bin/darwin64

config:
	@echo "get Dependencies"
	go env
	go get gopkg.in/cheggaaa/pb.v1
	go get github.com/jessevdk/go-flags
	go get github.com/olekukonko/ts
	go get github.com/cihub/seelog
	go get github.com/parnurzeal/gorequest

dist: cross-build package

dist-all: all package

package:
	@echo "Packaging"
	tar cfz bin/darwin64.tar.gz bin/darwin64
	tar cfz bin/linux64.tar.gz bin/linux64
	tar cfz bin/windows64.tar.gz bin/windows64



cross-compile:
	@echo "Prepare Cross Compiling"
	cd $(GOROOT)/src && GOOS=windows GOARCH=amd64 ./make.bash --no-clean
	cd $(GOROOT)/src && GOOS=darwin  GOARCH=amd64 ./make.bash --no-clean 2> /dev/null 1> /dev/null
	cd $(GOROOT)/src && GOOS=linux  GOARCH=amd64 ./make.bash --no-clean 2> /dev/null 1> /dev/null

	cd $(CWD)
