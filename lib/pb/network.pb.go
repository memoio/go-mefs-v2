// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: network.proto

package pb

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type NetMessage_MsgType int32

const (
	NetMessage_Err        NetMessage_MsgType = 0
	NetMessage_SayHello   NetMessage_MsgType = 1
	NetMessage_Get        NetMessage_MsgType = 2
	NetMessage_Put        NetMessage_MsgType = 3
	NetMessage_PutPeer    NetMessage_MsgType = 4
	NetMessage_SubmitTX   NetMessage_MsgType = 5
	NetMessage_PutSegment NetMessage_MsgType = 6
	NetMessage_GetSegment NetMessage_MsgType = 7
	NetMessage_GetPubKey  NetMessage_MsgType = 8
	// data message; user<->provider; offline ops
	NetMessage_AskPrice     NetMessage_MsgType = 10
	NetMessage_CreateOrder  NetMessage_MsgType = 11
	NetMessage_DoneOrder    NetMessage_MsgType = 12
	NetMessage_CreateSeq    NetMessage_MsgType = 13
	NetMessage_FinishSeq    NetMessage_MsgType = 14
	NetMessage_OrderSegment NetMessage_MsgType = 15
	NetMessage_GetOrder     NetMessage_MsgType = 16
	NetMessage_GetSeq       NetMessage_MsgType = 17
	NetMessage_FixSeq       NetMessage_MsgType = 18
)

var NetMessage_MsgType_name = map[int32]string{
	0:  "Err",
	1:  "SayHello",
	2:  "Get",
	3:  "Put",
	4:  "PutPeer",
	5:  "SubmitTX",
	6:  "PutSegment",
	7:  "GetSegment",
	8:  "GetPubKey",
	10: "AskPrice",
	11: "CreateOrder",
	12: "DoneOrder",
	13: "CreateSeq",
	14: "FinishSeq",
	15: "OrderSegment",
	16: "GetOrder",
	17: "GetSeq",
	18: "FixSeq",
}

var NetMessage_MsgType_value = map[string]int32{
	"Err":          0,
	"SayHello":     1,
	"Get":          2,
	"Put":          3,
	"PutPeer":      4,
	"SubmitTX":     5,
	"PutSegment":   6,
	"GetSegment":   7,
	"GetPubKey":    8,
	"AskPrice":     10,
	"CreateOrder":  11,
	"DoneOrder":    12,
	"CreateSeq":    13,
	"FinishSeq":    14,
	"OrderSegment": 15,
	"GetOrder":     16,
	"GetSeq":       17,
	"FixSeq":       18,
}

func (x NetMessage_MsgType) String() string {
	return proto.EnumName(NetMessage_MsgType_name, int32(x))
}

func (NetMessage_MsgType) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{0, 0}
}

type EventMessage_Type int32

const (
	EventMessage_Unknown EventMessage_Type = 0
	EventMessage_GetPeer EventMessage_Type = 1
	EventMessage_PutPeer EventMessage_Type = 2
	EventMessage_LfsMeta EventMessage_Type = 11
)

var EventMessage_Type_name = map[int32]string{
	0:  "Unknown",
	1:  "GetPeer",
	2:  "PutPeer",
	11: "LfsMeta",
}

var EventMessage_Type_value = map[string]int32{
	"Unknown": 0,
	"GetPeer": 1,
	"PutPeer": 2,
	"LfsMeta": 11,
}

func (x EventMessage_Type) String() string {
	return proto.EnumName(EventMessage_Type_name, int32(x))
}

func (EventMessage_Type) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{1, 0}
}

// trans over network
// persist value for tx in kv op db; verify sign before put
type NetMessage struct {
	Header               *NetMessage_MsgHeader `protobuf:"bytes,1,opt,name=header,proto3" json:"header,omitempty"`
	Data                 *NetMessage_MsgData   `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
	XXX_NoUnkeyedLiteral struct{}              `json:"-"`
	XXX_unrecognized     []byte                `json:"-"`
	XXX_sizecache        int32                 `json:"-"`
}

func (m *NetMessage) Reset()         { *m = NetMessage{} }
func (m *NetMessage) String() string { return proto.CompactTextString(m) }
func (*NetMessage) ProtoMessage()    {}
func (*NetMessage) Descriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{0}
}
func (m *NetMessage) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *NetMessage) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_NetMessage.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *NetMessage) XXX_Merge(src proto.Message) {
	xxx_messageInfo_NetMessage.Merge(m, src)
}
func (m *NetMessage) XXX_Size() int {
	return m.Size()
}
func (m *NetMessage) XXX_DiscardUnknown() {
	xxx_messageInfo_NetMessage.DiscardUnknown(m)
}

var xxx_messageInfo_NetMessage proto.InternalMessageInfo

func (m *NetMessage) GetHeader() *NetMessage_MsgHeader {
	if m != nil {
		return m.Header
	}
	return nil
}

func (m *NetMessage) GetData() *NetMessage_MsgData {
	if m != nil {
		return m.Data
	}
	return nil
}

type NetMessage_MsgHeader struct {
	Version              uint32             `protobuf:"varint,1,opt,name=version,proto3" json:"version,omitempty"`
	Type                 NetMessage_MsgType `protobuf:"varint,2,opt,name=type,proto3,enum=pb.NetMessage_MsgType" json:"type,omitempty"`
	From                 uint64             `protobuf:"varint,3,opt,name=from,proto3" json:"from,omitempty"`
	Seq                  uint64             `protobuf:"varint,4,opt,name=seq,proto3" json:"seq,omitempty"`
	XXX_NoUnkeyedLiteral struct{}           `json:"-"`
	XXX_unrecognized     []byte             `json:"-"`
	XXX_sizecache        int32              `json:"-"`
}

func (m *NetMessage_MsgHeader) Reset()         { *m = NetMessage_MsgHeader{} }
func (m *NetMessage_MsgHeader) String() string { return proto.CompactTextString(m) }
func (*NetMessage_MsgHeader) ProtoMessage()    {}
func (*NetMessage_MsgHeader) Descriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{0, 0}
}
func (m *NetMessage_MsgHeader) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *NetMessage_MsgHeader) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_NetMessage_MsgHeader.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *NetMessage_MsgHeader) XXX_Merge(src proto.Message) {
	xxx_messageInfo_NetMessage_MsgHeader.Merge(m, src)
}
func (m *NetMessage_MsgHeader) XXX_Size() int {
	return m.Size()
}
func (m *NetMessage_MsgHeader) XXX_DiscardUnknown() {
	xxx_messageInfo_NetMessage_MsgHeader.DiscardUnknown(m)
}

var xxx_messageInfo_NetMessage_MsgHeader proto.InternalMessageInfo

func (m *NetMessage_MsgHeader) GetVersion() uint32 {
	if m != nil {
		return m.Version
	}
	return 0
}

func (m *NetMessage_MsgHeader) GetType() NetMessage_MsgType {
	if m != nil {
		return m.Type
	}
	return NetMessage_Err
}

func (m *NetMessage_MsgHeader) GetFrom() uint64 {
	if m != nil {
		return m.From
	}
	return 0
}

func (m *NetMessage_MsgHeader) GetSeq() uint64 {
	if m != nil {
		return m.Seq
	}
	return 0
}

type NetMessage_MsgData struct {
	MsgInfo              []byte   `protobuf:"bytes,1,opt,name=msgInfo,proto3" json:"msgInfo,omitempty"`
	Sign                 []byte   `protobuf:"bytes,2,opt,name=sign,proto3" json:"sign,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *NetMessage_MsgData) Reset()         { *m = NetMessage_MsgData{} }
func (m *NetMessage_MsgData) String() string { return proto.CompactTextString(m) }
func (*NetMessage_MsgData) ProtoMessage()    {}
func (*NetMessage_MsgData) Descriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{0, 1}
}
func (m *NetMessage_MsgData) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *NetMessage_MsgData) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_NetMessage_MsgData.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *NetMessage_MsgData) XXX_Merge(src proto.Message) {
	xxx_messageInfo_NetMessage_MsgData.Merge(m, src)
}
func (m *NetMessage_MsgData) XXX_Size() int {
	return m.Size()
}
func (m *NetMessage_MsgData) XXX_DiscardUnknown() {
	xxx_messageInfo_NetMessage_MsgData.DiscardUnknown(m)
}

var xxx_messageInfo_NetMessage_MsgData proto.InternalMessageInfo

func (m *NetMessage_MsgData) GetMsgInfo() []byte {
	if m != nil {
		return m.MsgInfo
	}
	return nil
}

func (m *NetMessage_MsgData) GetSign() []byte {
	if m != nil {
		return m.Sign
	}
	return nil
}

// pubsub message content
type EventMessage struct {
	Type                 EventMessage_Type `protobuf:"varint,1,opt,name=type,proto3,enum=pb.EventMessage_Type" json:"type,omitempty"`
	Data                 []byte            `protobuf:"bytes,2,opt,name=data,proto3" json:"data,omitempty"`
	XXX_NoUnkeyedLiteral struct{}          `json:"-"`
	XXX_unrecognized     []byte            `json:"-"`
	XXX_sizecache        int32             `json:"-"`
}

func (m *EventMessage) Reset()         { *m = EventMessage{} }
func (m *EventMessage) String() string { return proto.CompactTextString(m) }
func (*EventMessage) ProtoMessage()    {}
func (*EventMessage) Descriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{1}
}
func (m *EventMessage) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *EventMessage) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_EventMessage.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *EventMessage) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EventMessage.Merge(m, src)
}
func (m *EventMessage) XXX_Size() int {
	return m.Size()
}
func (m *EventMessage) XXX_DiscardUnknown() {
	xxx_messageInfo_EventMessage.DiscardUnknown(m)
}

var xxx_messageInfo_EventMessage proto.InternalMessageInfo

func (m *EventMessage) GetType() EventMessage_Type {
	if m != nil {
		return m.Type
	}
	return EventMessage_Unknown
}

func (m *EventMessage) GetData() []byte {
	if m != nil {
		return m.Data
	}
	return nil
}

type PutPeerInfo struct {
	RoleID               uint64   `protobuf:"varint,1,opt,name=roleID,proto3" json:"roleID,omitempty"`
	NetID                []byte   `protobuf:"bytes,2,opt,name=netID,proto3" json:"netID,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *PutPeerInfo) Reset()         { *m = PutPeerInfo{} }
func (m *PutPeerInfo) String() string { return proto.CompactTextString(m) }
func (*PutPeerInfo) ProtoMessage()    {}
func (*PutPeerInfo) Descriptor() ([]byte, []int) {
	return fileDescriptor_8571034d60397816, []int{2}
}
func (m *PutPeerInfo) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *PutPeerInfo) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_PutPeerInfo.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *PutPeerInfo) XXX_Merge(src proto.Message) {
	xxx_messageInfo_PutPeerInfo.Merge(m, src)
}
func (m *PutPeerInfo) XXX_Size() int {
	return m.Size()
}
func (m *PutPeerInfo) XXX_DiscardUnknown() {
	xxx_messageInfo_PutPeerInfo.DiscardUnknown(m)
}

var xxx_messageInfo_PutPeerInfo proto.InternalMessageInfo

func (m *PutPeerInfo) GetRoleID() uint64 {
	if m != nil {
		return m.RoleID
	}
	return 0
}

func (m *PutPeerInfo) GetNetID() []byte {
	if m != nil {
		return m.NetID
	}
	return nil
}

func init() {
	proto.RegisterEnum("pb.NetMessage_MsgType", NetMessage_MsgType_name, NetMessage_MsgType_value)
	proto.RegisterEnum("pb.EventMessage_Type", EventMessage_Type_name, EventMessage_Type_value)
	proto.RegisterType((*NetMessage)(nil), "pb.NetMessage")
	proto.RegisterType((*NetMessage_MsgHeader)(nil), "pb.NetMessage.MsgHeader")
	proto.RegisterType((*NetMessage_MsgData)(nil), "pb.NetMessage.MsgData")
	proto.RegisterType((*EventMessage)(nil), "pb.EventMessage")
	proto.RegisterType((*PutPeerInfo)(nil), "pb.PutPeerInfo")
}

func init() { proto.RegisterFile("network.proto", fileDescriptor_8571034d60397816) }

var fileDescriptor_8571034d60397816 = []byte{
	// 502 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x6c, 0x93, 0xc1, 0x8e, 0x12, 0x4f,
	0x10, 0xc6, 0x19, 0x98, 0x65, 0x76, 0x6b, 0x06, 0xb6, 0xff, 0x9d, 0xbf, 0x9b, 0xc9, 0x1e, 0xc8,
	0x86, 0xd3, 0xea, 0x81, 0x18, 0x3d, 0x98, 0xe8, 0x49, 0x65, 0x97, 0x25, 0x8a, 0x4e, 0x86, 0x35,
	0xf1, 0x3a, 0x48, 0xc1, 0x4e, 0x80, 0x6e, 0xe8, 0x6e, 0x16, 0xb9, 0x79, 0xf5, 0x0d, 0xbc, 0xf9,
	0x3a, 0x1e, 0x7d, 0x04, 0x83, 0x2f, 0x62, 0xaa, 0x18, 0x70, 0x8d, 0xde, 0xea, 0xd7, 0xf5, 0xf5,
	0xf7, 0xf5, 0x74, 0xf5, 0x40, 0x4d, 0xa1, 0x5b, 0x69, 0x33, 0x69, 0xcd, 0x8d, 0x76, 0x5a, 0x96,
	0xe7, 0x83, 0xe6, 0x57, 0x1f, 0xe0, 0x0d, 0xba, 0x1e, 0x5a, 0x9b, 0x8d, 0x51, 0x3e, 0x84, 0xea,
	0x0d, 0x66, 0x43, 0x34, 0xb1, 0x77, 0xe6, 0x9d, 0x87, 0x8f, 0xe2, 0xd6, 0x7c, 0xd0, 0xfa, 0xdd,
	0x6f, 0xf5, 0xec, 0xf8, 0x8a, 0xfb, 0x69, 0xa1, 0x93, 0x0f, 0xc0, 0x1f, 0x66, 0x2e, 0x8b, 0xcb,
	0xac, 0x3f, 0xf9, 0x5b, 0xdf, 0xce, 0x5c, 0x96, 0xb2, 0xe6, 0x74, 0x05, 0x47, 0x7b, 0x03, 0x19,
	0x43, 0x70, 0x8b, 0xc6, 0xe6, 0x5a, 0x71, 0x56, 0x2d, 0xdd, 0x21, 0x59, 0xba, 0xf5, 0x1c, 0xd9,
	0xb2, 0xfe, 0x2f, 0xcb, 0xeb, 0xf5, 0x1c, 0x53, 0xd6, 0x48, 0x09, 0xfe, 0xc8, 0xe8, 0x59, 0x5c,
	0x39, 0xf3, 0xce, 0xfd, 0x94, 0x6b, 0x29, 0xa0, 0x62, 0x71, 0x11, 0xfb, 0xbc, 0x44, 0xe5, 0xe9,
	0x13, 0x08, 0x8a, 0x93, 0x50, 0xec, 0xcc, 0x8e, 0xbb, 0x6a, 0xa4, 0x39, 0x36, 0x4a, 0x77, 0x48,
	0x56, 0x36, 0x1f, 0x2b, 0x8e, 0x8d, 0x52, 0xae, 0x9b, 0x9f, 0xca, 0xbc, 0x93, 0x02, 0x65, 0x00,
	0x95, 0x0b, 0x63, 0x44, 0x49, 0x46, 0x70, 0xd8, 0xcf, 0xd6, 0x57, 0x38, 0x9d, 0x6a, 0xe1, 0xd1,
	0x72, 0x07, 0x9d, 0x28, 0x53, 0x91, 0x2c, 0x9d, 0xa8, 0xc8, 0x10, 0x82, 0x64, 0xe9, 0x12, 0x44,
	0x23, 0x7c, 0x16, 0x2f, 0x07, 0xb3, 0xdc, 0x5d, 0xbf, 0x17, 0x07, 0xb2, 0x0e, 0x90, 0x2c, 0x5d,
	0x1f, 0xc7, 0x33, 0x54, 0x4e, 0x54, 0x89, 0x3b, 0xb8, 0xe7, 0x40, 0xd6, 0xe0, 0xa8, 0x83, 0x2e,
	0x59, 0x0e, 0x5e, 0xe1, 0x5a, 0x1c, 0xd2, 0xe6, 0xe7, 0x76, 0x92, 0x98, 0xfc, 0x03, 0x0a, 0x90,
	0xc7, 0x10, 0xbe, 0x34, 0x98, 0x39, 0x7c, 0x6b, 0x86, 0x68, 0x44, 0x48, 0xea, 0xb6, 0x56, 0x05,
	0x46, 0x84, 0xdb, 0x7e, 0x1f, 0x17, 0xa2, 0x46, 0x78, 0x99, 0xab, 0xdc, 0xde, 0x10, 0xd6, 0xa5,
	0x80, 0x88, 0x85, 0xbb, 0xb0, 0x63, 0x72, 0xef, 0xa0, 0xdb, 0xee, 0x16, 0x12, 0xa0, 0xca, 0x47,
	0x59, 0x88, 0xff, 0xa8, 0xbe, 0xcc, 0x3f, 0x52, 0x2d, 0x9b, 0x9f, 0x3d, 0x88, 0x2e, 0x6e, 0x51,
	0xed, 0xdf, 0xc8, 0xfd, 0x62, 0x3c, 0x1e, 0x8f, 0xe7, 0x1e, 0x8d, 0xe7, 0x6e, 0xbf, 0xf5, 0xe7,
	0x74, 0xf6, 0x8f, 0x23, 0xda, 0x3e, 0x82, 0xe6, 0x53, 0xf0, 0xf9, 0x3a, 0x43, 0x08, 0xde, 0xa9,
	0x89, 0xd2, 0x2b, 0x25, 0x4a, 0x04, 0xf4, 0xdd, 0x74, 0x65, 0xde, 0xdd, 0xfb, 0x2b, 0x13, 0xbc,
	0x1e, 0xd9, 0x1e, 0xba, 0x4c, 0x84, 0xcd, 0x67, 0x10, 0x16, 0x1d, 0x9e, 0xd8, 0x09, 0x54, 0x8d,
	0x9e, 0x62, 0xb7, 0xcd, 0x67, 0xf1, 0xd3, 0x82, 0xe4, 0xff, 0x70, 0xa0, 0xd0, 0x75, 0xdb, 0x45,
	0xee, 0x16, 0x5e, 0x88, 0x6f, 0x9b, 0x86, 0xf7, 0x7d, 0xd3, 0xf0, 0x7e, 0x6c, 0x1a, 0xde, 0x97,
	0x9f, 0x8d, 0xd2, 0xa0, 0xca, 0xff, 0xc1, 0xe3, 0x5f, 0x01, 0x00, 0x00, 0xff, 0xff, 0x58, 0x15,
	0xed, 0xec, 0x18, 0x03, 0x00, 0x00,
}

func (m *NetMessage) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *NetMessage) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *NetMessage) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if m.Data != nil {
		{
			size, err := m.Data.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintNetwork(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0x12
	}
	if m.Header != nil {
		{
			size, err := m.Header.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintNetwork(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *NetMessage_MsgHeader) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *NetMessage_MsgHeader) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *NetMessage_MsgHeader) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if m.Seq != 0 {
		i = encodeVarintNetwork(dAtA, i, uint64(m.Seq))
		i--
		dAtA[i] = 0x20
	}
	if m.From != 0 {
		i = encodeVarintNetwork(dAtA, i, uint64(m.From))
		i--
		dAtA[i] = 0x18
	}
	if m.Type != 0 {
		i = encodeVarintNetwork(dAtA, i, uint64(m.Type))
		i--
		dAtA[i] = 0x10
	}
	if m.Version != 0 {
		i = encodeVarintNetwork(dAtA, i, uint64(m.Version))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func (m *NetMessage_MsgData) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *NetMessage_MsgData) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *NetMessage_MsgData) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if len(m.Sign) > 0 {
		i -= len(m.Sign)
		copy(dAtA[i:], m.Sign)
		i = encodeVarintNetwork(dAtA, i, uint64(len(m.Sign)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.MsgInfo) > 0 {
		i -= len(m.MsgInfo)
		copy(dAtA[i:], m.MsgInfo)
		i = encodeVarintNetwork(dAtA, i, uint64(len(m.MsgInfo)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *EventMessage) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *EventMessage) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *EventMessage) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if len(m.Data) > 0 {
		i -= len(m.Data)
		copy(dAtA[i:], m.Data)
		i = encodeVarintNetwork(dAtA, i, uint64(len(m.Data)))
		i--
		dAtA[i] = 0x12
	}
	if m.Type != 0 {
		i = encodeVarintNetwork(dAtA, i, uint64(m.Type))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func (m *PutPeerInfo) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *PutPeerInfo) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *PutPeerInfo) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.XXX_unrecognized != nil {
		i -= len(m.XXX_unrecognized)
		copy(dAtA[i:], m.XXX_unrecognized)
	}
	if len(m.NetID) > 0 {
		i -= len(m.NetID)
		copy(dAtA[i:], m.NetID)
		i = encodeVarintNetwork(dAtA, i, uint64(len(m.NetID)))
		i--
		dAtA[i] = 0x12
	}
	if m.RoleID != 0 {
		i = encodeVarintNetwork(dAtA, i, uint64(m.RoleID))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintNetwork(dAtA []byte, offset int, v uint64) int {
	offset -= sovNetwork(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *NetMessage) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Header != nil {
		l = m.Header.Size()
		n += 1 + l + sovNetwork(uint64(l))
	}
	if m.Data != nil {
		l = m.Data.Size()
		n += 1 + l + sovNetwork(uint64(l))
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *NetMessage_MsgHeader) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Version != 0 {
		n += 1 + sovNetwork(uint64(m.Version))
	}
	if m.Type != 0 {
		n += 1 + sovNetwork(uint64(m.Type))
	}
	if m.From != 0 {
		n += 1 + sovNetwork(uint64(m.From))
	}
	if m.Seq != 0 {
		n += 1 + sovNetwork(uint64(m.Seq))
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *NetMessage_MsgData) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.MsgInfo)
	if l > 0 {
		n += 1 + l + sovNetwork(uint64(l))
	}
	l = len(m.Sign)
	if l > 0 {
		n += 1 + l + sovNetwork(uint64(l))
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *EventMessage) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Type != 0 {
		n += 1 + sovNetwork(uint64(m.Type))
	}
	l = len(m.Data)
	if l > 0 {
		n += 1 + l + sovNetwork(uint64(l))
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func (m *PutPeerInfo) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.RoleID != 0 {
		n += 1 + sovNetwork(uint64(m.RoleID))
	}
	l = len(m.NetID)
	if l > 0 {
		n += 1 + l + sovNetwork(uint64(l))
	}
	if m.XXX_unrecognized != nil {
		n += len(m.XXX_unrecognized)
	}
	return n
}

func sovNetwork(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozNetwork(x uint64) (n int) {
	return sovNetwork(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *NetMessage) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetwork
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: NetMessage: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: NetMessage: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Header", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthNetwork
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthNetwork
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Header == nil {
				m.Header = &NetMessage_MsgHeader{}
			}
			if err := m.Header.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Data", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthNetwork
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthNetwork
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Data == nil {
				m.Data = &NetMessage_MsgData{}
			}
			if err := m.Data.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipNetwork(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetwork
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *NetMessage_MsgHeader) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetwork
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: MsgHeader: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MsgHeader: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Version", wireType)
			}
			m.Version = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Version |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= NetMessage_MsgType(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field From", wireType)
			}
			m.From = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.From |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Seq", wireType)
			}
			m.Seq = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Seq |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipNetwork(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetwork
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *NetMessage_MsgData) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetwork
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: MsgData: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: MsgData: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field MsgInfo", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetwork
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetwork
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.MsgInfo = append(m.MsgInfo[:0], dAtA[iNdEx:postIndex]...)
			if m.MsgInfo == nil {
				m.MsgInfo = []byte{}
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Sign", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetwork
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetwork
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Sign = append(m.Sign[:0], dAtA[iNdEx:postIndex]...)
			if m.Sign == nil {
				m.Sign = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipNetwork(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetwork
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *EventMessage) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetwork
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: EventMessage: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: EventMessage: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Type", wireType)
			}
			m.Type = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Type |= EventMessage_Type(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Data", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetwork
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetwork
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Data = append(m.Data[:0], dAtA[iNdEx:postIndex]...)
			if m.Data == nil {
				m.Data = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipNetwork(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetwork
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *PutPeerInfo) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowNetwork
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: PutPeerInfo: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: PutPeerInfo: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field RoleID", wireType)
			}
			m.RoleID = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.RoleID |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field NetID", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthNetwork
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthNetwork
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.NetID = append(m.NetID[:0], dAtA[iNdEx:postIndex]...)
			if m.NetID == nil {
				m.NetID = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipNetwork(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthNetwork
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			m.XXX_unrecognized = append(m.XXX_unrecognized, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipNetwork(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowNetwork
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowNetwork
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthNetwork
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupNetwork
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthNetwork
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthNetwork        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowNetwork          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupNetwork = fmt.Errorf("proto: unexpected end of group")
)
