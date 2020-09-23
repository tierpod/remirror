# remirror
Caching proxy for various public things (arch linux, fedora, centos, and other misc. things)

To build, you need to have a working Go installation on your computer. (See https://golang.org/doc/install)

Just check out the repository and then run:

    go build .
    ./remirror

It defaults to cache it's files in /var/remirror and uses a hardcoded upstream mirror at Xmission at the moment.

I've got a config-hcl branch with lots of configuration improvements--- It will be merged to master soon.

See Also
--------
A cool person has made an Ansible Playbook: https://gitlab.com/ciphermail/debops.remirror

