.PHONY: all test clean

all: hello

hello: hello.c
	cc -Wall -O2 -o hello hello.c

test: hello
	./hello

clean:
	rm -f hello
