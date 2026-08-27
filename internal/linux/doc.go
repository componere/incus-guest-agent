// Package linux implements the [github.com/componere/incus-guest-agent/internal/agent]
// ports with Linux syscalls.
//
// Device discovery enumerates block devices matching [DeviceGlob]. Staging
// mounts a candidate read-only as iso9660, copies the five required files into
// a private tmpfs, and unmounts the medium before the agent starts. Process
// supervision enables child-subreaper mode, starts the staged binary in its
// own process group, and reaps every descendant.
package linux
