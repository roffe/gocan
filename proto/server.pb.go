// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.28.0
// source: proto/server.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type CANFrameTypeEnum int32

const (
	CANFrameTypeEnum_Incoming                 CANFrameTypeEnum = 0
	CANFrameTypeEnum_Outgoing                 CANFrameTypeEnum = 1
	CANFrameTypeEnum_OutgoingResponseRequired CANFrameTypeEnum = 2
)

// Enum value maps for CANFrameTypeEnum.
var (
	CANFrameTypeEnum_name = map[int32]string{
		0: "Incoming",
		1: "Outgoing",
		2: "OutgoingResponseRequired",
	}
	CANFrameTypeEnum_value = map[string]int32{
		"Incoming":                 0,
		"Outgoing":                 1,
		"OutgoingResponseRequired": 2,
	}
)

func (x CANFrameTypeEnum) Enum() *CANFrameTypeEnum {
	p := new(CANFrameTypeEnum)
	*p = x
	return p
}

func (x CANFrameTypeEnum) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (CANFrameTypeEnum) Descriptor() protoreflect.EnumDescriptor {
	return file_proto_server_proto_enumTypes[0].Descriptor()
}

func (CANFrameTypeEnum) Type() protoreflect.EnumType {
	return &file_proto_server_proto_enumTypes[0]
}

func (x CANFrameTypeEnum) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Do not use.
func (x *CANFrameTypeEnum) UnmarshalJSON(b []byte) error {
	num, err := protoimpl.X.UnmarshalJSONEnum(x.Descriptor(), b)
	if err != nil {
		return err
	}
	*x = CANFrameTypeEnum(num)
	return nil
}

// Deprecated: Use CANFrameTypeEnum.Descriptor instead.
func (CANFrameTypeEnum) EnumDescriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{0}
}

type Adapters struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Adapters []*AdapterInfo `protobuf:"bytes,1,rep,name=adapters" json:"adapters,omitempty"`
}

func (x *Adapters) Reset() {
	*x = Adapters{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Adapters) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Adapters) ProtoMessage() {}

func (x *Adapters) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Adapters.ProtoReflect.Descriptor instead.
func (*Adapters) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{0}
}

func (x *Adapters) GetAdapters() []*AdapterInfo {
	if x != nil {
		return x.Adapters
	}
	return nil
}

type AdapterInfo struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name              *string              `protobuf:"bytes,1,req,name=Name" json:"Name,omitempty"`
	Description       *string              `protobuf:"bytes,2,opt,name=Description" json:"Description,omitempty"`
	Capabilities      *AdapterCapabilities `protobuf:"bytes,3,req,name=Capabilities" json:"Capabilities,omitempty"`
	RequireSerialPort *bool                `protobuf:"varint,4,req,name=RequireSerialPort" json:"RequireSerialPort,omitempty"`
}

func (x *AdapterInfo) Reset() {
	*x = AdapterInfo{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AdapterInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AdapterInfo) ProtoMessage() {}

func (x *AdapterInfo) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AdapterInfo.ProtoReflect.Descriptor instead.
func (*AdapterInfo) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{1}
}

func (x *AdapterInfo) GetName() string {
	if x != nil && x.Name != nil {
		return *x.Name
	}
	return ""
}

func (x *AdapterInfo) GetDescription() string {
	if x != nil && x.Description != nil {
		return *x.Description
	}
	return ""
}

func (x *AdapterInfo) GetCapabilities() *AdapterCapabilities {
	if x != nil {
		return x.Capabilities
	}
	return nil
}

func (x *AdapterInfo) GetRequireSerialPort() bool {
	if x != nil && x.RequireSerialPort != nil {
		return *x.RequireSerialPort
	}
	return false
}

type SerialPorts struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ports []*SerialPort `protobuf:"bytes,1,rep,name=ports" json:"ports,omitempty"`
}

func (x *SerialPorts) Reset() {
	*x = SerialPorts{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SerialPorts) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SerialPorts) ProtoMessage() {}

func (x *SerialPorts) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SerialPorts.ProtoReflect.Descriptor instead.
func (*SerialPorts) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{2}
}

func (x *SerialPorts) GetPorts() []*SerialPort {
	if x != nil {
		return x.Ports
	}
	return nil
}

type SerialPort struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name        *string `protobuf:"bytes,1,req,name=Name" json:"Name,omitempty"`
	Description *string `protobuf:"bytes,2,opt,name=Description" json:"Description,omitempty"`
}

func (x *SerialPort) Reset() {
	*x = SerialPort{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SerialPort) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SerialPort) ProtoMessage() {}

func (x *SerialPort) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SerialPort.ProtoReflect.Descriptor instead.
func (*SerialPort) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{3}
}

func (x *SerialPort) GetName() string {
	if x != nil && x.Name != nil {
		return *x.Name
	}
	return ""
}

func (x *SerialPort) GetDescription() string {
	if x != nil && x.Description != nil {
		return *x.Description
	}
	return ""
}

type AdapterCapabilities struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	HSCAN *bool `protobuf:"varint,1,opt,name=HSCAN" json:"HSCAN,omitempty"`
	SWCAN *bool `protobuf:"varint,2,opt,name=SWCAN" json:"SWCAN,omitempty"`
	KLine *bool `protobuf:"varint,3,opt,name=KLine" json:"KLine,omitempty"`
}

func (x *AdapterCapabilities) Reset() {
	*x = AdapterCapabilities{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AdapterCapabilities) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AdapterCapabilities) ProtoMessage() {}

func (x *AdapterCapabilities) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AdapterCapabilities.ProtoReflect.Descriptor instead.
func (*AdapterCapabilities) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{4}
}

func (x *AdapterCapabilities) GetHSCAN() bool {
	if x != nil && x.HSCAN != nil {
		return *x.HSCAN
	}
	return false
}

func (x *AdapterCapabilities) GetSWCAN() bool {
	if x != nil && x.SWCAN != nil {
		return *x.SWCAN
	}
	return false
}

func (x *AdapterCapabilities) GetKLine() bool {
	if x != nil && x.KLine != nil {
		return *x.KLine
	}
	return false
}

type Command struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Data []byte `protobuf:"bytes,1,req,name=data" json:"data,omitempty"`
}

func (x *Command) Reset() {
	*x = Command{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Command) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Command) ProtoMessage() {}

func (x *Command) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Command.ProtoReflect.Descriptor instead.
func (*Command) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{5}
}

func (x *Command) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type CommandResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Data []byte `protobuf:"bytes,1,req,name=data" json:"data,omitempty"`
}

func (x *CommandResponse) Reset() {
	*x = CommandResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CommandResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CommandResponse) ProtoMessage() {}

func (x *CommandResponse) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CommandResponse.ProtoReflect.Descriptor instead.
func (*CommandResponse) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{6}
}

func (x *CommandResponse) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

type CANFrameType struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	FrameType *CANFrameTypeEnum `protobuf:"varint,1,req,name=FrameType,enum=CANFrameTypeEnum" json:"FrameType,omitempty"`
	Responses *uint32           `protobuf:"varint,2,req,name=Responses" json:"Responses,omitempty"`
}

func (x *CANFrameType) Reset() {
	*x = CANFrameType{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CANFrameType) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CANFrameType) ProtoMessage() {}

func (x *CANFrameType) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CANFrameType.ProtoReflect.Descriptor instead.
func (*CANFrameType) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{7}
}

func (x *CANFrameType) GetFrameType() CANFrameTypeEnum {
	if x != nil && x.FrameType != nil {
		return *x.FrameType
	}
	return CANFrameTypeEnum_Incoming
}

func (x *CANFrameType) GetResponses() uint32 {
	if x != nil && x.Responses != nil {
		return *x.Responses
	}
	return 0
}

type CANFrame struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Extended  *bool         `protobuf:"varint,1,opt,name=extended" json:"extended,omitempty"`
	Id        *uint32       `protobuf:"varint,2,req,name=id" json:"id,omitempty"`
	Data      []byte        `protobuf:"bytes,3,req,name=data" json:"data,omitempty"`
	FrameType *CANFrameType `protobuf:"bytes,4,req,name=frameType" json:"frameType,omitempty"`
}

func (x *CANFrame) Reset() {
	*x = CANFrame{}
	if protoimpl.UnsafeEnabled {
		mi := &file_proto_server_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CANFrame) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CANFrame) ProtoMessage() {}

func (x *CANFrame) ProtoReflect() protoreflect.Message {
	mi := &file_proto_server_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CANFrame.ProtoReflect.Descriptor instead.
func (*CANFrame) Descriptor() ([]byte, []int) {
	return file_proto_server_proto_rawDescGZIP(), []int{8}
}

func (x *CANFrame) GetExtended() bool {
	if x != nil && x.Extended != nil {
		return *x.Extended
	}
	return false
}

func (x *CANFrame) GetId() uint32 {
	if x != nil && x.Id != nil {
		return *x.Id
	}
	return 0
}

func (x *CANFrame) GetData() []byte {
	if x != nil {
		return x.Data
	}
	return nil
}

func (x *CANFrame) GetFrameType() *CANFrameType {
	if x != nil {
		return x.FrameType
	}
	return nil
}

var File_proto_server_proto protoreflect.FileDescriptor

var file_proto_server_proto_rawDesc = []byte{
	0x0a, 0x12, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1b, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d, 0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x22, 0x34, 0x0a, 0x08, 0x41, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x73, 0x12, 0x28, 0x0a,
	0x08, 0x61, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x0c, 0x2e, 0x41, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x08, 0x61,
	0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x73, 0x22, 0xab, 0x01, 0x0a, 0x0b, 0x41, 0x64, 0x61, 0x70,
	0x74, 0x65, 0x72, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x12, 0x0a, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x18,
	0x01, 0x20, 0x02, 0x28, 0x09, 0x52, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x44,
	0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0b, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x38, 0x0a,
	0x0c, 0x43, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x69, 0x65, 0x73, 0x18, 0x03, 0x20,
	0x02, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x41, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x43, 0x61, 0x70,
	0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x69, 0x65, 0x73, 0x52, 0x0c, 0x43, 0x61, 0x70, 0x61, 0x62,
	0x69, 0x6c, 0x69, 0x74, 0x69, 0x65, 0x73, 0x12, 0x2c, 0x0a, 0x11, 0x52, 0x65, 0x71, 0x75, 0x69,
	0x72, 0x65, 0x53, 0x65, 0x72, 0x69, 0x61, 0x6c, 0x50, 0x6f, 0x72, 0x74, 0x18, 0x04, 0x20, 0x02,
	0x28, 0x08, 0x52, 0x11, 0x52, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x53, 0x65, 0x72, 0x69, 0x61,
	0x6c, 0x50, 0x6f, 0x72, 0x74, 0x22, 0x30, 0x0a, 0x0b, 0x53, 0x65, 0x72, 0x69, 0x61, 0x6c, 0x50,
	0x6f, 0x72, 0x74, 0x73, 0x12, 0x21, 0x0a, 0x05, 0x70, 0x6f, 0x72, 0x74, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x0b, 0x2e, 0x53, 0x65, 0x72, 0x69, 0x61, 0x6c, 0x50, 0x6f, 0x72, 0x74,
	0x52, 0x05, 0x70, 0x6f, 0x72, 0x74, 0x73, 0x22, 0x42, 0x0a, 0x0a, 0x53, 0x65, 0x72, 0x69, 0x61,
	0x6c, 0x50, 0x6f, 0x72, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20,
	0x02, 0x28, 0x09, 0x52, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x44, 0x65, 0x73,
	0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b,
	0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x22, 0x57, 0x0a, 0x13, 0x41,
	0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x43, 0x61, 0x70, 0x61, 0x62, 0x69, 0x6c, 0x69, 0x74, 0x69,
	0x65, 0x73, 0x12, 0x14, 0x0a, 0x05, 0x48, 0x53, 0x43, 0x41, 0x4e, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x05, 0x48, 0x53, 0x43, 0x41, 0x4e, 0x12, 0x14, 0x0a, 0x05, 0x53, 0x57, 0x43, 0x41,
	0x4e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08, 0x52, 0x05, 0x53, 0x57, 0x43, 0x41, 0x4e, 0x12, 0x14,
	0x0a, 0x05, 0x4b, 0x4c, 0x69, 0x6e, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x05, 0x4b,
	0x4c, 0x69, 0x6e, 0x65, 0x22, 0x1d, 0x0a, 0x07, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x12,
	0x12, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x01, 0x20, 0x02, 0x28, 0x0c, 0x52, 0x04, 0x64,
	0x61, 0x74, 0x61, 0x22, 0x25, 0x0a, 0x0f, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x01,
	0x20, 0x02, 0x28, 0x0c, 0x52, 0x04, 0x64, 0x61, 0x74, 0x61, 0x22, 0x5d, 0x0a, 0x0c, 0x43, 0x41,
	0x4e, 0x46, 0x72, 0x61, 0x6d, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x2f, 0x0a, 0x09, 0x46, 0x72,
	0x61, 0x6d, 0x65, 0x54, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x02, 0x28, 0x0e, 0x32, 0x11, 0x2e,
	0x43, 0x41, 0x4e, 0x46, 0x72, 0x61, 0x6d, 0x65, 0x54, 0x79, 0x70, 0x65, 0x45, 0x6e, 0x75, 0x6d,
	0x52, 0x09, 0x46, 0x72, 0x61, 0x6d, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x73, 0x18, 0x02, 0x20, 0x02, 0x28, 0x0d, 0x52, 0x09,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x73, 0x22, 0x77, 0x0a, 0x08, 0x43, 0x41, 0x4e,
	0x46, 0x72, 0x61, 0x6d, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x65, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x08, 0x65, 0x78, 0x74, 0x65, 0x6e, 0x64, 0x65,
	0x64, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x02, 0x20, 0x02, 0x28, 0x0d, 0x52, 0x02, 0x69,
	0x64, 0x12, 0x12, 0x0a, 0x04, 0x64, 0x61, 0x74, 0x61, 0x18, 0x03, 0x20, 0x02, 0x28, 0x0c, 0x52,
	0x04, 0x64, 0x61, 0x74, 0x61, 0x12, 0x2b, 0x0a, 0x09, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x54, 0x79,
	0x70, 0x65, 0x18, 0x04, 0x20, 0x02, 0x28, 0x0b, 0x32, 0x0d, 0x2e, 0x43, 0x41, 0x4e, 0x46, 0x72,
	0x61, 0x6d, 0x65, 0x54, 0x79, 0x70, 0x65, 0x52, 0x09, 0x66, 0x72, 0x61, 0x6d, 0x65, 0x54, 0x79,
	0x70, 0x65, 0x2a, 0x4c, 0x0a, 0x10, 0x43, 0x41, 0x4e, 0x46, 0x72, 0x61, 0x6d, 0x65, 0x54, 0x79,
	0x70, 0x65, 0x45, 0x6e, 0x75, 0x6d, 0x12, 0x0c, 0x0a, 0x08, 0x49, 0x6e, 0x63, 0x6f, 0x6d, 0x69,
	0x6e, 0x67, 0x10, 0x00, 0x12, 0x0c, 0x0a, 0x08, 0x4f, 0x75, 0x74, 0x67, 0x6f, 0x69, 0x6e, 0x67,
	0x10, 0x01, 0x12, 0x1c, 0x0a, 0x18, 0x4f, 0x75, 0x74, 0x67, 0x6f, 0x69, 0x6e, 0x67, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x64, 0x10, 0x02,
	0x32, 0xc8, 0x01, 0x0a, 0x05, 0x47, 0x6f, 0x63, 0x61, 0x6e, 0x12, 0x2b, 0x0a, 0x0b, 0x53, 0x65,
	0x6e, 0x64, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x12, 0x08, 0x2e, 0x43, 0x6f, 0x6d, 0x6d,
	0x61, 0x6e, 0x64, 0x1a, 0x10, 0x2e, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x38, 0x0a, 0x0e, 0x47, 0x65, 0x74, 0x53, 0x65,
	0x72, 0x69, 0x61, 0x6c, 0x50, 0x6f, 0x72, 0x74, 0x73, 0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74,
	0x79, 0x1a, 0x0c, 0x2e, 0x53, 0x65, 0x72, 0x69, 0x61, 0x6c, 0x50, 0x6f, 0x72, 0x74, 0x73, 0x22,
	0x00, 0x12, 0x32, 0x0a, 0x0b, 0x47, 0x65, 0x74, 0x41, 0x64, 0x61, 0x70, 0x74, 0x65, 0x72, 0x73,
	0x12, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x1a, 0x09, 0x2e, 0x41, 0x64, 0x61, 0x70, 0x74,
	0x65, 0x72, 0x73, 0x22, 0x00, 0x12, 0x24, 0x0a, 0x06, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x12,
	0x09, 0x2e, 0x43, 0x41, 0x4e, 0x46, 0x72, 0x61, 0x6d, 0x65, 0x1a, 0x09, 0x2e, 0x43, 0x41, 0x4e,
	0x46, 0x72, 0x61, 0x6d, 0x65, 0x22, 0x00, 0x28, 0x01, 0x30, 0x01, 0x42, 0x1e, 0x5a, 0x1c, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x6f, 0x66, 0x66, 0x65, 0x2f,
	0x67, 0x6f, 0x63, 0x61, 0x6e, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
}

var (
	file_proto_server_proto_rawDescOnce sync.Once
	file_proto_server_proto_rawDescData = file_proto_server_proto_rawDesc
)

func file_proto_server_proto_rawDescGZIP() []byte {
	file_proto_server_proto_rawDescOnce.Do(func() {
		file_proto_server_proto_rawDescData = protoimpl.X.CompressGZIP(file_proto_server_proto_rawDescData)
	})
	return file_proto_server_proto_rawDescData
}

var file_proto_server_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_proto_server_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_proto_server_proto_goTypes = []any{
	(CANFrameTypeEnum)(0),       // 0: CANFrameTypeEnum
	(*Adapters)(nil),            // 1: Adapters
	(*AdapterInfo)(nil),         // 2: AdapterInfo
	(*SerialPorts)(nil),         // 3: SerialPorts
	(*SerialPort)(nil),          // 4: SerialPort
	(*AdapterCapabilities)(nil), // 5: AdapterCapabilities
	(*Command)(nil),             // 6: Command
	(*CommandResponse)(nil),     // 7: CommandResponse
	(*CANFrameType)(nil),        // 8: CANFrameType
	(*CANFrame)(nil),            // 9: CANFrame
	(*emptypb.Empty)(nil),       // 10: google.protobuf.Empty
}
var file_proto_server_proto_depIdxs = []int32{
	2,  // 0: Adapters.adapters:type_name -> AdapterInfo
	5,  // 1: AdapterInfo.Capabilities:type_name -> AdapterCapabilities
	4,  // 2: SerialPorts.ports:type_name -> SerialPort
	0,  // 3: CANFrameType.FrameType:type_name -> CANFrameTypeEnum
	8,  // 4: CANFrame.frameType:type_name -> CANFrameType
	6,  // 5: Gocan.SendCommand:input_type -> Command
	10, // 6: Gocan.GetSerialPorts:input_type -> google.protobuf.Empty
	10, // 7: Gocan.GetAdapters:input_type -> google.protobuf.Empty
	9,  // 8: Gocan.Stream:input_type -> CANFrame
	7,  // 9: Gocan.SendCommand:output_type -> CommandResponse
	3,  // 10: Gocan.GetSerialPorts:output_type -> SerialPorts
	1,  // 11: Gocan.GetAdapters:output_type -> Adapters
	9,  // 12: Gocan.Stream:output_type -> CANFrame
	9,  // [9:13] is the sub-list for method output_type
	5,  // [5:9] is the sub-list for method input_type
	5,  // [5:5] is the sub-list for extension type_name
	5,  // [5:5] is the sub-list for extension extendee
	0,  // [0:5] is the sub-list for field type_name
}

func init() { file_proto_server_proto_init() }
func file_proto_server_proto_init() {
	if File_proto_server_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_proto_server_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*Adapters); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*AdapterInfo); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[2].Exporter = func(v any, i int) any {
			switch v := v.(*SerialPorts); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[3].Exporter = func(v any, i int) any {
			switch v := v.(*SerialPort); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[4].Exporter = func(v any, i int) any {
			switch v := v.(*AdapterCapabilities); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[5].Exporter = func(v any, i int) any {
			switch v := v.(*Command); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[6].Exporter = func(v any, i int) any {
			switch v := v.(*CommandResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[7].Exporter = func(v any, i int) any {
			switch v := v.(*CANFrameType); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_proto_server_proto_msgTypes[8].Exporter = func(v any, i int) any {
			switch v := v.(*CANFrame); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_proto_server_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_proto_server_proto_goTypes,
		DependencyIndexes: file_proto_server_proto_depIdxs,
		EnumInfos:         file_proto_server_proto_enumTypes,
		MessageInfos:      file_proto_server_proto_msgTypes,
	}.Build()
	File_proto_server_proto = out.File
	file_proto_server_proto_rawDesc = nil
	file_proto_server_proto_goTypes = nil
	file_proto_server_proto_depIdxs = nil
}
