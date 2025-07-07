PHONY: run

run:
	export $(shell cat .env | xargs)
	go run ./cmd/server/main.go ./cmd/server/wire_gen.go