package config

import (
	"os"

	"github.com/popelev/level2/internal/core"
)

// ApplyDeviceEnvOverlay fills OPC lab secrets from env when YAML omits them.
func ApplyDeviceEnvOverlay(d *core.Device) {
	if d == nil {
		return
	}
	if ep := os.Getenv("PLC_OPC_ENDPOINT"); ep != "" {
		d.Endpoint = ep
	}
	if u := os.Getenv("OPC_UA_USERNAME"); u != "" {
		d.Username = u
	}
	if p := os.Getenv("OPC_UA_PASSWORD"); p != "" {
		d.Password = p
	}
}
