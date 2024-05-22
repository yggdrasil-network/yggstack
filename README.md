# Yggstack - Yggdrasil as SOCKS proxy / port forwarder

[![Build status](https://github.com/yggdrasil-network/yggstack/actions/workflows/trunk.yml/badge.svg)](https://github.com/yggdrasil-network/yggstack/actions/workflows/trunk.yml)

## Introduction

Yggdrasil is an early-stage implementation of a fully end-to-end encrypted IPv6
network. It is lightweight, self-arranging, supported on multiple platforms and
allows pretty much any IPv6-capable application to communicate securely with
other Yggdrasil nodes. Yggdrasil does not require you to have IPv6 Internet
connectivity - it also works over IPv4.

Mainline Yggdrasil implementation uses virtual network interface (TUN) to deliver traffic.
While this setup is very powerful and flexible, several use cases are not covered:

* Systems without TUN adapter support
* System without root / administrator access
* Web browser access

Yggstack fills the gap by providing SOCKS5 proxy server and TCP port forwarder
functionality similar to TOR router. It also can serve as a standalone network node
to connect network segments.

## Supported Platforms

Yggdrasil works on a number of platforms, including Linux, macOS, Ubiquiti
EdgeRouter, VyOS, Windows, FreeBSD, OpenBSD and OpenWrt.

Please see our [Installation](https://yggdrasil-network.github.io/installation.html)
page for more information. You may also find other platform-specific wrappers, scripts
or tools in the `contrib` folder.

## Downloading

Bleeding-edge binaries can be downloaded via [trunk release](https://github.com/yggdrasil-network/yggstack/releases/tag/trunk)

Tagged releases provide packages similar to Yggdrasil.

## Building

If you want to build from source, as opposed to installing one of the pre-built
packages:

1. Install [Go](https://golang.org) (requires Go 1.17 or later)
2. Clone this repository
2. Run `./build`

Note that you can cross-compile for other platforms and architectures by
specifying the `GOOS` and `GOARCH` environment variables, e.g. `GOOS=windows
./build` or `GOOS=linux GOARCH=mipsle ./build`.

## Running

### Generate configuration

To generate static configuration, either generate a HJSON file (human-friendly,
complete with comments):

```
./yggstack -genconf > /path/to/yggdrasil.conf
```

... or generate a plain JSON file (which is easy to manipulate
programmatically):

```
./yggstack -genconf -json > /path/to/yggdrasil.conf
```

You will need to edit the `yggdrasil.conf` file to add or remove peers, modify
other configuration such as listen addresses or multicast addresses, etc.

### Run Yggstack

To run SOCKS proxy server listening on local port 1080 using generated configuration:

```
./yggstack -useconffile /path/to/yggdrasil.conf -socks 127.0.0.1:1080
```

To run SOCKS proxy server listening on UNIX socket file `/tmp/yggstack.sock`:

```
./yggstack -useconffile /path/to/yggdrasil.conf -socks /tmp/yggstack.sock
```

To expose network services (like a Web server) listening on local port 8080 to Yggdrasil
network address at port 80:

```
./yggstack -useconffile /path/to/yggdrasil.conf -exposetcp 80:127.0.0.1:8080
```

To run as a standalone node without SOCKS server or TCP port forwarding:
```
./yggstack -useconffile /path/to/yggdrasil.conf
```

To run in auto-configuration mode (which will use sane defaults and random keys
at each startup, instead of using a static configuration file):

```
./yggstack -autoconf -socks 127.0.0.1:1080
```

Unlike mainline Yggdrasil, Yggstack does NOT require privileged access.
You can even run several Yggstack instances with different configurations
on the same OS and user!

### pk.ygg DNS resolver

One unique feature of Yggstack is built-in DNS resolver functionality using
`<publickey>.pk.ygg` format.

For example, HowToYgg website (whose public key is `d40d4a7153cf288ea28f1865f6cfe95143a478b5c8c9e7cb002a0633d10a53eb`)
can be accessed by any Web browser supporting SOCKS servers
via  `http://d40d4a7153cf288ea28f1865f6cfe95143a478b5c8c9e7cb002a0633d10a53eb.pk.ygg`

You can even use cURL with Yggstack:

```
curl -x socks5h://127.0.0.1:1080 http://d40d4a7153cf288ea28f1865f6cfe95143a478b5c8c9e7cb002a0633d10a53eb.pk.ygg
```

## Documentation

Documentation is available [on our website](https://yggdrasil-network.github.io).

- [Installing Yggdrasil](https://yggdrasil-network.github.io/installation.html)
- [Configuring Yggdrasil](https://yggdrasil-network.github.io/configuration.html)
- [Frequently asked questions](https://yggdrasil-network.github.io/faq.html)
- [Version changelog](CHANGELOG.md)

## Community

Feel free to join us on our [Matrix
channel](https://matrix.to/#/#yggdrasil:matrix.org) at `#yggdrasil:matrix.org`
or in the `#yggdrasil` IRC channel on [libera.chat](https://libera.chat).

## License

This code is released under the terms of the LGPLv3, but with an added exception
that was shamelessly taken from [godeb](https://github.com/niemeyer/godeb).
Under certain circumstances, this exception permits distribution of binaries
that are (statically or dynamically) linked with this code, without requiring
the distribution of Minimal Corresponding Source or Minimal Application Code.
For more details, see: [LICENSE](LICENSE).
