package linux

import "time"

const (
	// DeviceGlob matches optical block devices probed for Incus media.
	DeviceGlob = "/dev/sr*"
	// RuntimeRoot is the private runtime directory used for media and staging mounts.
	RuntimeRoot = "/var/run/incus-guest-agent"
	// MediaName is the subdirectory under [RuntimeRoot] used as the iso9660 mountpoint.
	MediaName = "media"
	// StageName is the subdirectory under [RuntimeRoot] used as the tmpfs staging area.
	StageName = "agent"
	// TmpfsSource is the mount source label used for the staging tmpfs.
	TmpfsSource = "incus-guest-agent"
	// TmpfsData is the tmpfs mount data string.
	TmpfsData = "mode=0700,size=50M"
	// PollInterval is the delay between retryable media probes.
	PollInterval = 2 * time.Second
	// ShutdownGrace is how long SIGTERM is given before SIGKILL.
	ShutdownGrace = 10 * time.Second
	// KillGrace is how long SIGKILL is given before supervision fails.
	KillGrace = 2 * time.Second
	// executableBits is the permission mask that marks a file executable.
	executableBits = 0o111
	// copyTempSuffix is appended to a destination name while a copy is in progress.
	copyTempSuffix = ".tmp"
	// agentBinaryName is the staged executable basename.
	agentBinaryName = "incus-agent"
	// fileAgentConf is the required agent configuration basename.
	fileAgentConf = "agent.conf"
	// fileAgentCrt is the required agent certificate basename.
	fileAgentCrt = "agent.crt"
	// fileAgentKey is the required agent private-key basename.
	fileAgentKey = "agent.key"
	// fileServerCrt is the required server certificate basename.
	fileServerCrt = "server.crt"
)

// RequiredFiles returns the five Incus guest-agent media basenames in copy order.
func RequiredFiles() []string {
	return []string{
		agentBinaryName,
		fileAgentConf,
		fileAgentCrt,
		fileAgentKey,
		fileServerCrt,
	}
}

// StagePath returns the tmpfs staging directory under [RuntimeRoot].
func StagePath() string {
	return RuntimeRoot + "/" + StageName
}
