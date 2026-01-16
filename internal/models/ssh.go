package models

// SSHShutdownConfig holds SSH shutdown configuration.
type SSHShutdownConfig struct {
	Host          string
	Port          int
	Username      string
	PrivateKey    []byte // loaded from file path
	KeyPath       string // path to key file
	ShutdownDelay int    // seconds before shutdown (Linux: minutes, Windows: seconds)
	OS            string // "linux" (default) or "windows"
}

// SSHResult holds the result of an SSH operation.
type SSHResult struct {
	CommandRun bool
	Output     string
	Error      error
}
