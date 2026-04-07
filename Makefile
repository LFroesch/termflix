build:
	go build -o tf
cp:
	cp tf ~/.local/bin/

install: build cp
