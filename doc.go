// Package vsock provides access to Linux VM sockets (AF_VSOCK) for
// communication between a hypervisor and its virtual machines.
//
// The types in this package implement interfaces provided by package net and
// may be used in applications that expect a net.Listener or net.Conn.
//
//   - *Addr implements net.Addr
//   - *Conn implements net.Conn
//   - *Listener implements net.Listener
//
// Stability
//
// At this time, package vsock is in a pre-v1.0.0 state. Changes are being made
// which may impact the exported API of this package and others in its ecosystem.
//
// If you depend on this package in your application, please use Go modules when
// building your application.
package vsock
