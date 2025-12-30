package passthru

import "C"
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/bendikro/dl"
)

type PassThru struct {
	lib                     dl.DL
	passThruReadVersionProc func(uint32, uintptr, uintptr, uintptr) uint32
	passThruOpen            func(string, *uint32) uint32
	passThruClose           func(uint32) uint32
	passThruConnect         func(uint32, uint32, uint32, uint32, *uint32) uint32
	passThruDisconnect      func(uint32) uint32
	passThruReadMsgs        func(uint32, *PassThruMsg, *uint32, uint32) uint32
	passThruWriteMsgs       func(uint32, *PassThruMsg, *uint32, uint32) uint32
	passThruStartMsgFilter  func(uint32, uint32, *PassThruMsg, *PassThruMsg, *PassThruMsg, *uint32) uint32
	passThruIoctl           func(uint32, uint32, ...interface{}) uint32
	passThruGetLastError    func(uintptr) uint32
}

func New(libName string) (*PassThru, error) {
	lib, err := dl.Open(libName, 0)
	if err != nil {
		return nil, err
	}
	var Version func(uint32, uintptr, uintptr, uintptr) uint32
	err = lib.Sym("PassThruReadVersion", &Version)
	if err != nil {
		return nil, err
	}
	var Open func(string, *uint32) uint32
	err = lib.Sym("PassThruOpen", &Open)
	if err != nil {
		return nil, err
	}
	var Close func(deviceID uint32) uint32
	err = lib.Sym("PassThruClose", &Close)
	if err != nil {
		return nil, err
	}
	var Connect func(uint32, uint32, uint32, uint32, *uint32) uint32
	err = lib.Sym("PassThruConnect", &Connect)
	if err != nil {
		return nil, err
	}
	var Disconnect func(uint32) uint32
	err = lib.Sym("PassThruDisconnect", &Disconnect)
	if err != nil {
		return nil, err
	}
	var ReadMsgs func(uint32, *PassThruMsg, *uint32, uint32) uint32
	err = lib.Sym("PassThruReadMsgs", &ReadMsgs)
	if err != nil {
		return nil, err
	}
	var WriteMsgs func(uint32, *PassThruMsg, *uint32, uint32) uint32
	err = lib.Sym("PassThruWriteMsgs", &WriteMsgs)
	if err != nil {
		return nil, err
	}
	var StartMsgFilter func(uint32, uint32, *PassThruMsg, *PassThruMsg, *PassThruMsg, *uint32) uint32
	err = lib.Sym("PassThruStartMsgFilter", &StartMsgFilter)
	if err != nil {
		return nil, err
	}
	var Ioctl func(uint32, uint32, ...interface{}) uint32
	err = lib.Sym("PassThruIoctl", &Ioctl)
	if err != nil {
		return nil, err
	}
	var GetLastError func(uintptr) uint32
	err = lib.Sym("PassThruGetLastError", &GetLastError)
	if err != nil {
		return nil, err
	}

	return &PassThru{
		passThruReadVersionProc: Version,
		passThruOpen:            Open,
		passThruClose:           Close,
		passThruConnect:         Connect,
		passThruDisconnect:      Disconnect,
		passThruReadMsgs:        ReadMsgs,
		passThruWriteMsgs:       WriteMsgs,
		passThruStartMsgFilter:  StartMsgFilter,
		passThruIoctl:           Ioctl,
		passThruGetLastError:    GetLastError,
	}, nil
}

// Close long PassThruClose(unsigned long DeviceID);
func (j *PassThru) Close() error {
	return j.lib.Close()
}

// PassThruOpen long PassThruOpen(void* pName, unsigned long *pDeviceID);
func (j *PassThru) PassThruOpen(deviceName string, pDeviceID *uint32) error {
	ret := j.passThruOpen(deviceName, pDeviceID)
	return CheckError(ret)
}

// PassThruClose long PassThruClose(unsigned long DeviceID);
func (j *PassThru) PassThruClose(deviceID uint32) error {
	ret := j.passThruClose(deviceID)
	return CheckError(ret)
}

// PassThruConnect long PassThruConnect(unsigned long DeviceID, unsigned long ProtocolID, unsigned long Flags, unsigned long BaudRate, unsigned long *pChannelID);
func (j *PassThru) PassThruConnect(deviceID uint32, protocolID uint32, flags uint32, baudRate uint32, pChannelID *uint32) error {
	ret := j.passThruConnect(
		deviceID,
		protocolID,
		flags,
		baudRate,
		pChannelID,
	)
	return CheckError(ret)
}

// PassThruDisconnect long PassThruDisconnect(unsigned long ChannelID);
func (j *PassThru) PassThruDisconnect(channelID uint32) error {
	ret := j.passThruDisconnect(channelID)
	return CheckError(ret)
}

func (j *PassThru) PassThruReadMsg(channelID uint32, pMsg *PassThruMsg, timeout uint32) (uint32, error) {
	pNumMsgs := uint32(1)
	// long PassThruReadMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
	ret := j.passThruReadMsgs(
		channelID,
		pMsg,
		&pNumMsgs,
		timeout,
	)
	if err := CheckError(ret); err != nil {
		if str, err2 := j.PassThruGetLastError(); err2 == nil {
			return 0, fmt.Errorf("%s: %w", str, err)
		} else {
			return 0, err
		}
	}
	return pNumMsgs, nil
}

// PassThruReadMsgs long PassThruReadMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
func (j *PassThru) PassThruReadMsgs(channelID uint32, pMsg *PassThruMsg, pNumMsgs *uint32, timeout uint32) error {
	ret := j.passThruReadMsgs(
		channelID,
		pMsg,
		pNumMsgs,
		timeout,
	)
	if err := CheckError(ret); err != nil {
		if str, err2 := j.PassThruGetLastError(); err2 == nil {
			return fmt.Errorf("%s: %w", str, err)
		} else {
			return err
		}
	}
	return nil
}

// PassThruWriteMsgs long PassThruWriteMsgs(unsigned long ChannelID, PassThruMsg *pMsg, unsigned long *pNumMsgs, unsigned long Timeout);
func (j *PassThru) PassThruWriteMsgs(channelID uint32, pMsg *PassThruMsg, pNumMsgs *uint32, timeout uint32) error {
	ret := j.passThruWriteMsgs(
		channelID,
		pMsg,
		pNumMsgs,
		timeout,
	)
	return CheckError(ret)
}

// PassThruStartMsgFilter long PassThruStartMsgFilter(unsigned long ChannelID, unsigned long FilterType, PassThruMsg *pMaskMsg, PassThruMsg *pPatternMsg, PassThruMsg *pFlowControlMsg, unsigned long *pMsgID);
func (j *PassThru) PassThruStartMsgFilter(channelID uint32, filterType uint32, pMaskMsg, pPatternMsg, pFlowControlMsg *PassThruMsg, pMsgID *uint32) error {
	ret := j.passThruStartMsgFilter(
		channelID,
		filterType,
		pMaskMsg,
		pPatternMsg,
		pFlowControlMsg,
		pMsgID,
	)
	return CheckError(ret)
}

// PassThruReadVersion long PassThruReadVersion(unsigned long DeviceID, char *pFirmwareVersion, char *pDllVersion, char *pApiVersion);
func (j *PassThru) PassThruReadVersion(deviceID uint32) (string, string, string, error) {
	var pFirmwareVersion [80]byte
	var pDllVersion [80]byte
	var pApiVersion [80]byte

	ret := j.passThruReadVersionProc(
		deviceID,
		uintptr(unsafe.Pointer(&pFirmwareVersion)),
		uintptr(unsafe.Pointer(&pDllVersion)),
		uintptr(unsafe.Pointer(&pApiVersion)),
	)

	if err := CheckError(ret); err != nil {
		return "", "", "", err
	}

	return string(bytes.Trim(pFirmwareVersion[:], "\x00")), string(bytes.Trim(pDllVersion[:], "\x00")), string(bytes.Trim(pApiVersion[:], "\x00")), nil
}

// PassThruIoctl long PassThruIoctl(unsigned long HandleID, unsigned long IoctlID, void *pInput, void *pOutput);
func (j *PassThru) PassThruIoctl(handleID uint32, ioctlID uint32, opts ...interface{}) error {
	switch ioctlID {
	case SET_CONFIG, GET_CONFIG:
		ret := j.passThruIoctl(handleID,
			ioctlID,
			opts[0].(*SCONFIG_LIST),
			0,
		)
		return CheckError(ret)
	case CLEAR_MSG_FILTERS, CLEAR_RX_BUFFER, CLEAR_TX_BUFFER:
		ret := j.passThruIoctl(handleID, ioctlID, 0, 0)
		return CheckError(ret)
	case FAST_INIT:
		ret := j.passThruIoctl(handleID, ioctlID,
			uintptr(unsafe.Pointer(opts[0].(*PassThruMsg))),
			uintptr(unsafe.Pointer(opts[1].(*PassThruMsg))),
		)
		return CheckError(ret)
	}
	return ErrNotSupported
}

// PassThruGetLastError long PassThruGetLastError(char *pErrorDescription);
func (j *PassThru) PassThruGetLastError() (string, error) {
	var pErrorDescription [80]byte
	ret := j.passThruGetLastError(uintptr(unsafe.Pointer(&pErrorDescription)))
	return string(bytes.Trim(pErrorDescription[:], "\x00")), CheckError(ret)
}

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

func FindConfigFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func FindDLLs() (_ string, libs []J2534DLL) {
	home, _ := os.UserHomeDir()
	configDir := home + "/.passthru/"
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return
	}
	configFiles, err := FindConfigFiles(configDir)

	for _, file := range configFiles {
		if _, err := os.Stat(file); errors.Is(err, os.ErrNotExist) {
			return
		}
		jsonFile, _ := os.Open(file)

		defer func(jsonFile *os.File) {
			err := jsonFile.Close()
			if err != nil {
				return
			}
		}(jsonFile)

		byteValue, _ := io.ReadAll(jsonFile)
		var config J2534Config
		err = json.Unmarshal(byteValue, &config)
		if err != nil {
			return
		}

		if strings.HasPrefix(config.FUNCTIONLIB, "~/") {
			config.FUNCTIONLIB = filepath.Join(home, config.FUNCTIONLIB[2:])
		}

		name := config.VENDOR + " " + config.NAME
		if _, err := os.Stat(config.FUNCTIONLIB); err == nil && filepath.Ext(config.FUNCTIONLIB) == ".so" {
			var capabilities = Capabilities{
				SWCANPS:  config.SWCANPS,
				CAN:      config.CAN,
				CANPS:    config.CANPS,
				ISO15765: config.ISO15765,
				ISO9141:  config.ISO9141,
				ISO14230: config.ISO14230,
			}
			libs = append(libs, J2534DLL{Name: name, FunctionLibrary: config.FUNCTIONLIB, Capabilities: capabilities})
		}
	}
	return
}
