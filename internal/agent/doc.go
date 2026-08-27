// Package agent orchestrates Incus guest-agent discovery, staging, and
// supervision without performing filesystem, mount, process, or clock I/O.
//
// The package owns the domain type [DevicePath] and the consumer-side ports
// [DeviceFinder], [StageManager], [AgentProcess], and [Waiter]. Production
// constructors accept those interfaces and return a concrete [Service].
package agent
