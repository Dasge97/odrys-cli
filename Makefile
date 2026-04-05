run:
	go run ./cmd/odrys

build:
	mkdir -p dist
	go build -o dist/odrys ./cmd/odrys

legacy-tui:
	node src/cli.js tui
