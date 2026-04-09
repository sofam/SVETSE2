.PHONY: build test vet lint clean docker linux

build:
	go build -o svetse2 .

test:
	go test -count=1 ./...

vet:
	go vet ./...

lint: vet
	@echo "OK"

linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o svetse2-linux .

docker:
	docker build -t svetse2 .

clean:
	rm -f svetse2 svetse2-linux
