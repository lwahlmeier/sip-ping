sip-ping
========

Send a SIP OPTIONS request to a server over ws/wss.

example
-------

    sip-ping -addr wss://my-server

install
-------

Assuming you have [setup Go](https://golang.org/doc/install):

    go get github.com/watsoncj/sip-ping

building
--------

Uses docker for isolated/repeatable binary builds.

    ./build.sh
