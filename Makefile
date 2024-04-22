BINARY_NAME=go-luxpower-timescaledb

build:
	go build -o bin/${BINARY_NAME} cmd/main.go

clean:
	go clean
	rm bin/${BINARY_NAME}
