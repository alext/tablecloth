
SERVERS = simple_server double_listen v1/server v2/server

all: $(SERVERS)

v%/server.go: simple_server.go
	mkdir -p v$*
	sed 's/Hello/Hello from v$*/' $< > $@

%: %.go
	go build -o $@ $<

.PHONY: all
