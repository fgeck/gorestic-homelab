package models

import "time"

// WOLConfig holds Wake-on-LAN configuration.
type WOLConfig struct {
	MACAddress    string
	BroadcastIP   string
	PollURL       string        // URL to poll until target machine is ready
	Timeout       time.Duration // max time to wait for target
	PollInterval  time.Duration // how often to poll the URL
	StabilizeWait time.Duration // wait after target responds
}

// WOLResult holds the result of a Wake-on-LAN operation.
type WOLResult struct {
	PacketSent   bool
	TargetReady  bool
	WaitDuration time.Duration
	Error        error
}
