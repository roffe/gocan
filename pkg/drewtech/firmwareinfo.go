package drewtech

import "fmt"

type Version struct {
	Major, Minor, Patch, Build byte
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.Major, v.Minor, v.Patch, v.Build)
}

type FirmwareInfo struct {
	FW Version
	BL Version
}

func ExtractVersions(data []byte) FirmwareInfo {
	const fwOffset = 28
	const blOffset = 32

	return FirmwareInfo{
		FW: Version{
			Major: data[fwOffset+3],
			Minor: data[fwOffset+2],
			Patch: data[fwOffset+1],
			Build: data[fwOffset],
		},
		BL: Version{
			Major: data[blOffset+3],
			Minor: data[blOffset+2],
			Patch: data[blOffset+1],
			Build: data[blOffset],
		},
	}
}
