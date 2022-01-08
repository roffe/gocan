package t7

func LookupID(id uint32) string {
	if v, ok := PBusIDs[id]; ok {
		return "(P) " + v
	}
	if v, ok := IBusIDs[id]; ok {
		return "(I) " + v
	}
	return "??"
}

var PBusIDs = map[uint32]string{
	0x1A0: "Engine information",
	0x2F0: "Vehicle speed",
	0x280: "Pedals, reverse gear",
	0x338: "Trionic to SID text",
	0x358: "Trionic to SID text control",
	0x370: "Mileage",
	0x3A0: "Vehicle speed, MIU?",
	0x3B0: "Head lights",
	0x3E0: "Automatic Gearbox",
	0x530: "ACC",
	0x5C0: "Coolant temperature, air pressure",
	0x631: "??",
	0x6B1: "??",
	0x6B2: "??",
	0x740: "Security",
	0x750: "Security",
}

var IBusIDs = map[uint32]string{
	0x220: "Trionic data initialization",
	0x238: "Trionic data initialization reply",
	0x240: "Trionic data query",
	0x258: "Trionic data query reply",
	0x266: "Trionic reply acknowledgement",
	0x280: "Pedals, reverse gear",
	0x290: "Steering wheel and SID buttons",
	0x320: "Doors, central locking and seat belts",
	0x328: "SID audio text",
	0x32C: "ACC to SID text",
	0x32F: "TWICE to SID text",
	0x337: "SPA to SID text",
	0x348: "SID audio text control",
	0x34C: "ACC to SID text control",
	0x34F: "TWICE to SID text control",
	0x357: "SPA to SID text control",
	0x368: "SID text priority",
	0x380: "Audio RDS status",
	0x3B0: "Head lights",
	0x3C0: "CD Changer control",
	0x3C8: "CD Changer information",
	0x3E0: "Automatic Gearbox",
	0x410: "Light dimmer and light sensor",
	0x430: "SID beep request",
	0x439: "SPA distance",
	0x460: "Engine rpm and speed",
	0x4A0: "Steering wheel, VIN",
	0x520: "ACC, inside temperature",
	0x530: "ACC",
	0x590: "Position Seat Memory",
	0x5C0: "Coolant temperature, air pressure",
	0x630: "Fuel usage",
	0x640: "Mileage",
	0x6A1: "Audio head unit",
	0x6A2: "CD changer",
	0x720: "RDS time",
	0x730: "Clock",
	0x740: "Secutiry",
	0x750: "Security",
	0x7A0: "Outside temperature",
}
