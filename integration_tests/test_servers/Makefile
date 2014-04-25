
SERVERS = single_listen double_listen

all: $(SERVERS)

%: %.go
	go build -o $@ $<

.PHONY: all
