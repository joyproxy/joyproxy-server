# Requires Go 1.17+ on PATH
.PHONY: dist-linux
dist-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o dist/joyproxy-linux-amd64 ./cmd/joyproxy
	cd dist && tar -czvf joyproxy-centos7-linux-amd64.tar.gz joyproxy-linux-amd64 README-CentOS7.txt
