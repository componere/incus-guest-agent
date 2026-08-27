// Package main is the composition root for the Incus guest-agent wrapper.
//
// Normal execution runs the Linux supervisor. The only accepted arguments are
// --help/-h and --version/-v. Unknown arguments are written to stderr and the
// process exits nonzero.
package main
