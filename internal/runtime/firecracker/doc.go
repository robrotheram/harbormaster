// Package firecracker owns host admission, jailed VM lifecycle, bounded vsock
// relays and project-bound credential brokering. Unconfirmed termination retains
// capacity while revoking grants. It never falls back to another runtime.
package firecracker
