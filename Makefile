# The development version of clang is distributed as the 'clang' binary,
# while stable/released versions have a version number attached.
# Pin the default clang to a stable version.
CLANG ?= clang-14
CFLAGS := -O2 -g -Wall -Werror $(CFLAGS)

all: generate examples

format:
	find . -type f -name "*.c" | xargs clang-format -i

examples:
	cd pidfd-examples/share-socket && go build && cd ../graceful-upgrade && go build
	cd steer-multi-ports && go build

# $BPF_CLANG is used in go:generate invocations.
generate: export BPF_CLANG := $(CLANG)
generate: export BPF_CFLAGS := $(CFLAGS)
generate:
	cd steer-multi-ports && go generate ./...

clean:
	@rm -f ./steer-multi-ports/steer-multi-ports
	@rm -f ./pidfd-examples/graceful-upgrade/graceful-upgrade
	@rm -f ./pidfd-examples/share-socket/share-socket
