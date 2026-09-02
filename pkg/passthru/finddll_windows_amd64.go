package passthru

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func FindDLLs() (prefix string, dlls []J2534DLL) {
	prefix = "x64 "
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\PassThruSupport.04.04`, registry.READ)
	if err != nil {
		//log.Println(err)
		return
	}
	defer k.Close()

	// n <= 0 returns every subkey, so there is no separate Stat for the count.
	adapters, err := k.ReadSubKeyNames(-1)
	if err != nil {
		//log.Println(err)
		return
	}

	for _, adapter := range adapters {
		// Scoped so each driver's key is released this iteration; FindDLLs is
		// re-run on every device-list refresh.
		func() {
			k3, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\PassThruSupport.04.04\`+adapter, registry.QUERY_VALUE)
			if err != nil {
				return
			}
			defer k3.Close()

			var capabilities Capabilities
			name, _, err := k3.GetStringValue("Name")
			if err != nil {
				return
			}
			functionLibrary, _, err := k3.GetStringValue("FunctionLibrary")
			if err != nil {
				return
			}
			if val, _, err := k3.GetIntegerValue("CAN"); err == nil {
				capabilities.CAN = val == 1
			}
			if val, _, err := k3.GetIntegerValue("CAN_PS"); err == nil {
				capabilities.CANPS = val == 1
			}
			if val, _, err := k3.GetIntegerValue("ISO9141"); err == nil {
				capabilities.ISO9141 = val == 1
			}
			if val, _, err := k3.GetIntegerValue("ISO15765"); err == nil {
				capabilities.ISO15765 = val == 1
			}
			if val, _, err := k3.GetIntegerValue("ISO14230"); err == nil {
				capabilities.ISO14230 = val == 1
			}
			if val, _, err := k3.GetIntegerValue("SW_CAN_PS"); err == nil {
				capabilities.SWCANPS = val == 1 || strings.ToLower(name) == "tech2"
			} else {
				if strings.ToLower(name) == "tech2" {
					capabilities.SWCANPS = true
				}
			}
			dlls = append(dlls, J2534DLL{
				Name:            name,
				FunctionLibrary: functionLibrary,
				Capabilities:    capabilities,
			})
		}()
	}
	return
}
