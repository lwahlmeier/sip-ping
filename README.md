# sip-ping

Send a SIP OPTIONS request to a server over ws/wss or udp/tcp/tls.

## examples

### WSS:
```bash
sip-ping -addr wss://some.sip.server.com:443
```

### UDP:
```bash
sip-ping -addr udp://some.sip.server.com:5060
```

## install

Assuming you have [setup Go](https://golang.org/doc/install):

```bash
go get github.com/lwahlmeier/sip-ping
```

## building

Uses docker for isolated/repeatable binary builds.

```bash
./dockerBuild.sh
```
