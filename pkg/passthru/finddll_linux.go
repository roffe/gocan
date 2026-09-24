//go:build linux

package passthru

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// J2534Config is one ~/.passthru/<driver>.json file, the Linux stand-in for
// the PassThruSupport.04.04 registry key.
type J2534Config struct {
	CAN         bool   `json:"CAN"`
	CANPS       bool   `json:"CAN_PS"`
	ISO15765    bool   `json:"ISO15765"`
	ISO9141     bool   `json:"ISO9141"`
	ISO14230    bool   `json:"ISO14230"`
	SCIATRANS   bool   `json:"SCI_A_TRANS"`
	SCIAENGINE  bool   `json:"SCI_A_ENGINE"`
	SCIBTRANS   bool   `json:"SCI_B_TRANS"`
	SCIBENGINE  bool   `json:"SCI_B_ENGINE"`
	J1850VPW    bool   `json:"J1850VPW"`
	J1850PWM    bool   `json:"J1850PWM"`
	SWCANPS     bool   `json:"SW_CAN_PS"`
	FUNCTIONLIB string `json:"FUNCTION_LIB"`
	NAME        string `json:"NAME"`
	VENDOR      string `json:"VENDOR"`
	COMPORT     string `json:"COM-PORT"`
}

// FindDLLs lists the J2534 libraries described by ~/.passthru/*.json.
// A file that is unreadable, malformed or points at a missing library is
// skipped rather than aborting the scan. The prefix is always empty on
// Linux; there is no 32/64-bit split to label.
func FindDLLs() (prefix string, libs []J2534DLL) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil
	}
	files, _ := filepath.Glob(filepath.Join(home, ".passthru", "*.json"))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var cfg J2534Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if strings.HasPrefix(cfg.FUNCTIONLIB, "~/") {
			cfg.FUNCTIONLIB = filepath.Join(home, cfg.FUNCTIONLIB[2:])
		}
		if _, err := os.Stat(cfg.FUNCTIONLIB); err != nil {
			continue
		}
		libs = append(libs, J2534DLL{
			Name:            strings.TrimSpace(cfg.VENDOR + " " + cfg.NAME),
			FunctionLibrary: cfg.FUNCTIONLIB,
			Capabilities: Capabilities{
				CAN:      cfg.CAN,
				CANPS:    cfg.CANPS,
				ISO15765: cfg.ISO15765,
				ISO9141:  cfg.ISO9141,
				ISO14230: cfg.ISO14230,
				SWCANPS:  cfg.SWCANPS,
			},
		})
	}
	return "", libs
}
