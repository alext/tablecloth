
SERVERS = simple/server double_listen/server v1/server v2/server errorer/server

all: $(SERVERS)

clean:
	rm $(SERVERS)

v%/server.go: simple/server.go
	mkdir -p v$*
	sed 's/Hello/Hello from v$*/' $< > $@

%: %.go
	go build -o $@ $<

.PHONY: all
