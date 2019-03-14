# vsock [![Build Status](https://travis-ci.org/mdlayher/vsock.svg?branch=master)](https://travis-ci.org/mdlayher/vsock) [![GoDoc](https://godoc.org/github.com/mdlayher/vsock?status.svg)](https://godoc.org/github.com/mdlayher/vsock) [![Go Report Card](https://goreportcard.com/badge/github.com/mdlayher/vsock)](https://goreportcard.com/report/github.com/mdlayher/vsock)

Package `vsock` provides access to Linux VM sockets (`AF_VSOCK`) for
communication between a hypervisor and its virtual machines.  MIT Licensed.

For more information about VM sockets, check out my blog about
[Linux VM sockets in Go](https://medium.com/@mdlayher/linux-vm-sockets-in-go-ea11768e9e67).

## Stability

At this time, package `vsock` is in a pre-v1.0.0 state. Changes are being made
which may impact the exported API of this package and others in its ecosystem.

The general policy of this package is to only support the latest, stable version
of Go. Compatibility shims may be added for prior versions of Go on an as-needed
basis. If you would like to raise a concern, please [file an issue](https://github.com/mdlayher/vsock/issues/new).

**If you depend on this package in your applications, please vendor it or use Go
modules when building your application.**

## Requirements

To make use of VM sockets with QEMU and virtio-vsock, you must have:

- a Linux hypervisor with kernel 4.8+
- a Linux virtual machine on that hypervisor with kernel 4.8+
- QEMU 2.8+ on the hypervisor, running the virtual machine

Before using VM sockets, following modules must be removed on hypervisor:

- `modprobe -r vmw_vsock_vmci_transport`
- `modprobe -r vmw_vsock_virtio_transport_common`
- `modprobe -r vsock`

Once removed, `vhost_vsock` module needs to be enabled on hypervisor:

- `modprobe vhost_vsock`

On VM, you have to enable `vmw_vsock_virtio_transport` module.  This module should automatically load during boot when the vsock device is detected.

To utilize VM sockets, VM needs to be powered on with following `-device` flag:

- `-device vhost-vsock-pci,id=vhost-vsock-pci0,guest-cid=3`

Check out the
[QEMU wiki page on virtio-vsock](http://wiki.qemu-project.org/Features/VirtioVsock)
for more details.  More detail on setting up this environment will be provided
in the future.

## Usage

To try out VM sockets and see an example of how they work, see
[cmd/vscp](https://github.com/mdlayher/vsock/tree/master/cmd/vscp).
This command shows usage of the `vsock.ListenStream` and `vsock.DialStream`
APIs, and allows users to easily test VM sockets on their systems.
