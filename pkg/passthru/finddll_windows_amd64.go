package passthru

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

func FindDLLs() (prefix string, dlls []J2534DLL) {
	prefix = "x64 "
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\PassThruSupport.04.04`, registry.QUERY_VALUE)
	if err != nil {
		//log.Println(err)
		return
	}
	ki, err := k.Stat()
	if err != nil {
		//log.Println(err)
		return
	}

	if err := k.Close(); err != nil {
		//log.Println(err)
		return
	}

	k2, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\PassThruSupport.04.04`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		//log.Println(err)
		return
	}

	adapters, err := k2.ReadSubKeyNames(int(ki.SubKeyCount))
	if err != nil {
		//log.Println(err)
		return
	}

	var capabilities Capabilities
	for _, adapter := range adapters {
		k3, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\PassThruSupport.04.04\`+adapter, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		name, _, err := k3.GetStringValue("Name")
		if err != nil {
			continue
		}
		functionLibrary, _, err := k3.GetStringValue("FunctionLibrary")
		if err != nil {
			continue
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
		dlls = append(dlls, J2534DLL{Name: name, FunctionLibrary: functionLibrary, Capabilities: capabilities})
	}
	return
}
