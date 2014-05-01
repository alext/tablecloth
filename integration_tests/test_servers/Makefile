
SERVERS = single_listen double_listen v1/server v2/server

all: $(SERVERS)

%: %.go
	go build -o $@ $<

.PHONY: all
