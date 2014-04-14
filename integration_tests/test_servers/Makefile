
SERVERS = single_listen

all: $(SERVERS)

%: %.go
	go build -o $@ $<

.PHONY: all
