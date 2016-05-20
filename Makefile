SHELL=/bin/bash
CWD=$(shell pwd)
OLDGOPATH=${GOPATH}
NEWGOPATH:=${CWD}:${OLDGOPATH}
export GOPATH=$(NEWGOPATH)


build: clean config
	go build  -o bin/esmove

tar: build
	tar cfz bin/esmove.tar.gz bin/esmove

cross-build-all-platform: clean config
	go test
	GOOS=windows GOARCH=amd64     go build -o bin/windows64/esmove.exe
	GOOS=windows GOARCH=386       go build -o bin/windows32/esmove.exe
	GOOS=darwin  GOARCH=amd64     go build -o bin/darwin64/esmove
	GOOS=darwin  GOARCH=386       go build -o bin/darwin32/esmove
	GOOS=linux  GOARCH=amd64      go build -o bin/linux64/esmove
	GOOS=linux  GOARCH=386        go build -o bin/linux32/esmove
	GOOS=linux  GOARCH=arm        go build -o bin/linux_arm/esmove
	GOOS=freebsd  GOARCH=amd64    go build -o bin/freebsd64/esmove
	GOOS=freebsd  GOARCH=386      go build -o bin/freebsd32/esmove
	GOOS=netbsd  GOARCH=amd64     go build -o bin/netbsd64/esmove
	GOOS=netbsd  GOARCH=386       go build -o bin/netbsd32/esmove
	GOOS=openbsd  GOARCH=amd64    go build -o bin/openbsd64/esmove
	GOOS=openbsd  GOARCH=386      go build -o bin/openbsd32/esmove

cross-build: clean config
	go test
	GOOS=windows GOARCH=amd64     go build -o bin/windows64/esmove.exe
	GOOS=darwin  GOARCH=amd64     go build -o bin/darwin64/esmove
	GOOS=linux  GOARCH=amd64      go build -o bin/linux64/esmove

all: clean config cross-build

all-platform: clean config cross-build-all-platform

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

dist-all-platform: all-platform package-all-platform

package:
	@echo "Packaging"
	tar cfz 	 bin/windows64.tar.gz    bin/windows64/esmove.exe
	tar cfz 	 bin/darwin64.tar.gz      bin/darwin64/esmove
	tar cfz 	 bin/linux64.tar.gz      bin/linux64/esmove

package-all-platform:
	@echo "Packaging"
	tar cfz 	 bin/windows64.tar.gz    bin/windows64/esmove.exe
	tar cfz 	 bin/windows32.tar.gz    bin/windows32/esmove.exe
	tar cfz 	 bin/darwin64.tar.gz      bin/darwin64/esmove
	tar cfz 	 bin/darwin32.tar.gz      bin/darwin32/esmove
	tar cfz 	 bin/linux64.tar.gz      bin/linux64/esmove
	tar cfz 	 bin/linux32.tar.gz      bin/linux32/esmove
	tar cfz 	 bin/linux_arm.tar.gz     bin/linux_arm/esmove
	tar cfz 	 bin/freebsd64.tar.gz    bin/freebsd64/esmove
	tar cfz 	 bin/freebsd32.tar.gz    bin/freebsd32/esmove
	tar cfz 	 bin/netbsd64.tar.gz     bin/netbsd64/esmove
	tar cfz 	 bin/netbsd32.tar.gz     bin/netbsd32/esmove
	tar cfz 	 bin/openbsd64.tar.gz     bin/openbsd64/esmove
	tar cfz 	 bin/openbsd32.tar.gz     bin/openbsd32/esmove


cross-compile:
	@echo "Prepare Cross Compiling"
	cd $(GOROOT)/src && GOOS=windows GOARCH=amd64 ./make.bash --no-clean
	cd $(GOROOT)/src && GOOS=darwin  GOARCH=amd64 ./make.bash --no-clean 2> /dev/null 1> /dev/null
	cd $(GOROOT)/src && GOOS=linux  GOARCH=amd64 ./make.bash --no-clean 2> /dev/null 1> /dev/null

	cd $(CWD)
