/*
Copyright The TestGrid Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.0
// 	protoc        v3.21.1
// source: updater.proto

package updater

import (
	context "context"
	config "github.com/GoogleCloudPlatform/testgrid/pb/config"
	state "github.com/GoogleCloudPlatform/testgrid/pb/state"
	summary "github.com/GoogleCloudPlatform/testgrid/pb/summary"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// An identifier of a dashboard tab, i.e., the name of the dashboard and tab.
type DashboardTabIdentifier struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The name of a dashboard containing the dashboard tab.
	DashboardName string `protobuf:"bytes,1,opt,name=dashboard_name,json=dashboardName,proto3" json:"dashboard_name,omitempty"`
	// The dashboard tab to update.
	DashboardTab *config.DashboardTab `protobuf:"bytes,2,opt,name=dashboard_tab,json=dashboardTab,proto3" json:"dashboard_tab,omitempty"`
}

func (x *DashboardTabIdentifier) Reset() {
	*x = DashboardTabIdentifier{}
	if protoimpl.UnsafeEnabled {
		mi := &file_updater_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DashboardTabIdentifier) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DashboardTabIdentifier) ProtoMessage() {}

func (x *DashboardTabIdentifier) ProtoReflect() protoreflect.Message {
	mi := &file_updater_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DashboardTabIdentifier.ProtoReflect.Descriptor instead.
func (*DashboardTabIdentifier) Descriptor() ([]byte, []int) {
	return file_updater_proto_rawDescGZIP(), []int{0}
}

func (x *DashboardTabIdentifier) GetDashboardName() string {
	if x != nil {
		return x.DashboardName
	}
	return ""
}

func (x *DashboardTabIdentifier) GetDashboardTab() *config.DashboardTab {
	if x != nil {
		return x.DashboardTab
	}
	return nil
}

// An updater request to update a test group.
type UpdateRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The test group configuration to update.
	TestGroup *config.TestGroup `protobuf:"bytes,1,opt,name=test_group,json=testGroup,proto3" json:"test_group,omitempty"`
	// A list of dashboards tabs backed by the group being updated.
	DashboardTabIdentifiers []*DashboardTabIdentifier `protobuf:"bytes,2,rep,name=dashboard_tab_identifiers,json=dashboardTabIdentifiers,proto3" json:"dashboard_tab_identifiers,omitempty"`
}

func (x *UpdateRequest) Reset() {
	*x = UpdateRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_updater_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UpdateRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UpdateRequest) ProtoMessage() {}

func (x *UpdateRequest) ProtoReflect() protoreflect.Message {
	mi := &file_updater_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UpdateRequest.ProtoReflect.Descriptor instead.
func (*UpdateRequest) Descriptor() ([]byte, []int) {
	return file_updater_proto_rawDescGZIP(), []int{1}
}

func (x *UpdateRequest) GetTestGroup() *config.TestGroup {
	if x != nil {
		return x.TestGroup
	}
	return nil
}

func (x *UpdateRequest) GetDashboardTabIdentifiers() []*DashboardTabIdentifier {
	if x != nil {
		return x.DashboardTabIdentifiers
	}
	return nil
}

// An updater response after updating a test group.
type UpdateResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The time taken to perform the update in milliseconds.
	UpdateTimeMillis uint32 `protobuf:"varint,1,opt,name=update_time_millis,json=updateTimeMillis,proto3" json:"update_time_millis,omitempty"`
	// The size of the compressed output file in bytes.
	OutputSizeBytes uint32 `protobuf:"varint,2,opt,name=output_size_bytes,json=outputSizeBytes,proto3" json:"output_size_bytes,omitempty"`
	// Summaries of dashboard tabs associated with this group.
	DashboardTabSummaries []*summary.DashboardTabSummary `protobuf:"bytes,3,rep,name=dashboard_tab_summaries,json=dashboardTabSummaries,proto3" json:"dashboard_tab_summaries,omitempty"`
	// Metrics associated with the update for this group.
	UpdateEntry *state.UpdateInfo `protobuf:"bytes,4,opt,name=update_entry,json=updateEntry,proto3" json:"update_entry,omitempty"`
}

func (x *UpdateResponse) Reset() {
	*x = UpdateResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_updater_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *UpdateResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UpdateResponse) ProtoMessage() {}

func (x *UpdateResponse) ProtoReflect() protoreflect.Message {
	mi := &file_updater_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UpdateResponse.ProtoReflect.Descriptor instead.
func (*UpdateResponse) Descriptor() ([]byte, []int) {
	return file_updater_proto_rawDescGZIP(), []int{2}
}

func (x *UpdateResponse) GetUpdateTimeMillis() uint32 {
	if x != nil {
		return x.UpdateTimeMillis
	}
	return 0
}

func (x *UpdateResponse) GetOutputSizeBytes() uint32 {
	if x != nil {
		return x.OutputSizeBytes
	}
	return 0
}

func (x *UpdateResponse) GetDashboardTabSummaries() []*summary.DashboardTabSummary {
	if x != nil {
		return x.DashboardTabSummaries
	}
	return nil
}

func (x *UpdateResponse) GetUpdateEntry() *state.UpdateInfo {
	if x != nil {
		return x.UpdateEntry
	}
	return nil
}

var File_updater_proto protoreflect.FileDescriptor

var file_updater_proto_rawDesc = []byte{
	0x0a, 0x0d, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a,
	0x16, 0x70, 0x62, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x18, 0x70, 0x62, 0x2f, 0x73, 0x75, 0x6d, 0x6d,
	0x61, 0x72, 0x79, 0x2f, 0x73, 0x75, 0x6d, 0x6d, 0x61, 0x72, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x14, 0x70, 0x62, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x65, 0x2f, 0x73, 0x74, 0x61, 0x74,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x73, 0x0a, 0x16, 0x44, 0x61, 0x73, 0x68, 0x62,
	0x6f, 0x61, 0x72, 0x64, 0x54, 0x61, 0x62, 0x49, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x66, 0x69, 0x65,
	0x72, 0x12, 0x25, 0x0a, 0x0e, 0x64, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x5f, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x64, 0x61, 0x73, 0x68, 0x62,
	0x6f, 0x61, 0x72, 0x64, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x32, 0x0a, 0x0d, 0x64, 0x61, 0x73, 0x68,
	0x62, 0x6f, 0x61, 0x72, 0x64, 0x5f, 0x74, 0x61, 0x62, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x0d, 0x2e, 0x44, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x54, 0x61, 0x62, 0x52, 0x0c,
	0x64, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x54, 0x61, 0x62, 0x22, 0x8f, 0x01, 0x0a,
	0x0d, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x29,
	0x0a, 0x0a, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x0a, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x52, 0x09,
	0x74, 0x65, 0x73, 0x74, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x12, 0x53, 0x0a, 0x19, 0x64, 0x61, 0x73,
	0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x5f, 0x74, 0x61, 0x62, 0x5f, 0x69, 0x64, 0x65, 0x6e, 0x74,
	0x69, 0x66, 0x69, 0x65, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x44,
	0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x54, 0x61, 0x62, 0x49, 0x64, 0x65, 0x6e, 0x74,
	0x69, 0x66, 0x69, 0x65, 0x72, 0x52, 0x17, 0x64, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64,
	0x54, 0x61, 0x62, 0x49, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x66, 0x69, 0x65, 0x72, 0x73, 0x22, 0xe8,
	0x01, 0x0a, 0x0e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x12, 0x2c, 0x0a, 0x12, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65,
	0x5f, 0x6d, 0x69, 0x6c, 0x6c, 0x69, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x10, 0x75,
	0x70, 0x64, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x4d, 0x69, 0x6c, 0x6c, 0x69, 0x73, 0x12,
	0x2a, 0x0a, 0x11, 0x6f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x5f, 0x62,
	0x79, 0x74, 0x65, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0f, 0x6f, 0x75, 0x74, 0x70,
	0x75, 0x74, 0x53, 0x69, 0x7a, 0x65, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x4c, 0x0a, 0x17, 0x64,
	0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x5f, 0x74, 0x61, 0x62, 0x5f, 0x73, 0x75, 0x6d,
	0x6d, 0x61, 0x72, 0x69, 0x65, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x44,
	0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x54, 0x61, 0x62, 0x53, 0x75, 0x6d, 0x6d, 0x61,
	0x72, 0x79, 0x52, 0x15, 0x64, 0x61, 0x73, 0x68, 0x62, 0x6f, 0x61, 0x72, 0x64, 0x54, 0x61, 0x62,
	0x53, 0x75, 0x6d, 0x6d, 0x61, 0x72, 0x69, 0x65, 0x73, 0x12, 0x2e, 0x0a, 0x0c, 0x75, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x5f, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x0b, 0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x0b, 0x75, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x32, 0x67, 0x0a, 0x07, 0x55, 0x70, 0x64,
	0x61, 0x74, 0x65, 0x72, 0x12, 0x2b, 0x0a, 0x06, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x12, 0x0e,
	0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0f,
	0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22,
	0x00, 0x12, 0x2f, 0x0a, 0x0a, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x42, 0x75, 0x67, 0x73, 0x12,
	0x0e, 0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x0f, 0x2e, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x00, 0x42, 0x0d, 0x5a, 0x0b, 0x2f, 0x70, 0x62, 0x2f, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65,
	0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_updater_proto_rawDescOnce sync.Once
	file_updater_proto_rawDescData = file_updater_proto_rawDesc
)

func file_updater_proto_rawDescGZIP() []byte {
	file_updater_proto_rawDescOnce.Do(func() {
		file_updater_proto_rawDescData = protoimpl.X.CompressGZIP(file_updater_proto_rawDescData)
	})
	return file_updater_proto_rawDescData
}

var file_updater_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_updater_proto_goTypes = []interface{}{
	(*DashboardTabIdentifier)(nil),      // 0: DashboardTabIdentifier
	(*UpdateRequest)(nil),               // 1: UpdateRequest
	(*UpdateResponse)(nil),              // 2: UpdateResponse
	(*config.DashboardTab)(nil),         // 3: DashboardTab
	(*config.TestGroup)(nil),            // 4: TestGroup
	(*summary.DashboardTabSummary)(nil), // 5: DashboardTabSummary
	(*state.UpdateInfo)(nil),            // 6: UpdateInfo
}
var file_updater_proto_depIdxs = []int32{
	3, // 0: DashboardTabIdentifier.dashboard_tab:type_name -> DashboardTab
	4, // 1: UpdateRequest.test_group:type_name -> TestGroup
	0, // 2: UpdateRequest.dashboard_tab_identifiers:type_name -> DashboardTabIdentifier
	5, // 3: UpdateResponse.dashboard_tab_summaries:type_name -> DashboardTabSummary
	6, // 4: UpdateResponse.update_entry:type_name -> UpdateInfo
	1, // 5: Updater.Update:input_type -> UpdateRequest
	1, // 6: Updater.UpdateBugs:input_type -> UpdateRequest
	2, // 7: Updater.Update:output_type -> UpdateResponse
	2, // 8: Updater.UpdateBugs:output_type -> UpdateResponse
	7, // [7:9] is the sub-list for method output_type
	5, // [5:7] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_updater_proto_init() }
func file_updater_proto_init() {
	if File_updater_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_updater_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DashboardTabIdentifier); i {
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
		file_updater_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UpdateRequest); i {
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
		file_updater_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*UpdateResponse); i {
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
			RawDescriptor: file_updater_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_updater_proto_goTypes,
		DependencyIndexes: file_updater_proto_depIdxs,
		MessageInfos:      file_updater_proto_msgTypes,
	}.Build()
	File_updater_proto = out.File
	file_updater_proto_rawDesc = nil
	file_updater_proto_goTypes = nil
	file_updater_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// UpdaterClient is the client API for Updater service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type UpdaterClient interface {
	// Updates a test group state object either incrementally or by creating it.
	Update(ctx context.Context, in *UpdateRequest, opts ...grpc.CallOption) (*UpdateResponse, error)
	// Updates a bug state object for a given test group.
	UpdateBugs(ctx context.Context, in *UpdateRequest, opts ...grpc.CallOption) (*UpdateResponse, error)
}

type updaterClient struct {
	cc grpc.ClientConnInterface
}

func NewUpdaterClient(cc grpc.ClientConnInterface) UpdaterClient {
	return &updaterClient{cc}
}

func (c *updaterClient) Update(ctx context.Context, in *UpdateRequest, opts ...grpc.CallOption) (*UpdateResponse, error) {
	out := new(UpdateResponse)
	err := c.cc.Invoke(ctx, "/Updater/Update", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *updaterClient) UpdateBugs(ctx context.Context, in *UpdateRequest, opts ...grpc.CallOption) (*UpdateResponse, error) {
	out := new(UpdateResponse)
	err := c.cc.Invoke(ctx, "/Updater/UpdateBugs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdaterServer is the server API for Updater service.
type UpdaterServer interface {
	// Updates a test group state object either incrementally or by creating it.
	Update(context.Context, *UpdateRequest) (*UpdateResponse, error)
	// Updates a bug state object for a given test group.
	UpdateBugs(context.Context, *UpdateRequest) (*UpdateResponse, error)
}

// UnimplementedUpdaterServer can be embedded to have forward compatible implementations.
type UnimplementedUpdaterServer struct {
}

func (*UnimplementedUpdaterServer) Update(context.Context, *UpdateRequest) (*UpdateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Update not implemented")
}
func (*UnimplementedUpdaterServer) UpdateBugs(context.Context, *UpdateRequest) (*UpdateResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateBugs not implemented")
}

func RegisterUpdaterServer(s *grpc.Server, srv UpdaterServer) {
	s.RegisterService(&_Updater_serviceDesc, srv)
}

func _Updater_Update_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UpdaterServer).Update(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/Updater/Update",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UpdaterServer).Update(ctx, req.(*UpdateRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Updater_UpdateBugs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UpdaterServer).UpdateBugs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/Updater/UpdateBugs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UpdaterServer).UpdateBugs(ctx, req.(*UpdateRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Updater_serviceDesc = grpc.ServiceDesc{
	ServiceName: "Updater",
	HandlerType: (*UpdaterServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Update",
			Handler:    _Updater_Update_Handler,
		},
		{
			MethodName: "UpdateBugs",
			Handler:    _Updater_UpdateBugs_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "updater.proto",
}
