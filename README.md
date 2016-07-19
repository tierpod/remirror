# remirror
Caching proxy for various public things (arch linux, fedora, centos, and other misc. things)

To build, you need to have a working Go installation on your computer. (See https://golang.org/doc/install)

Just check out the repository and then run:

    go build .
    ./remirror

It defaults to cache it's files in /var/remirror and uses a hardcoded upstream mirror at Xmission at the moment.
