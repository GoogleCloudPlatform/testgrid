/*
Copyright 2021 The TestGrid Authors.

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

package config

import (
	"fmt"

	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type protoPath struct {
	name  string
	count map[string]int64
}

func (pp protoPath) countRecursive(fd protoreflect.FieldDescriptor, val protoreflect.Value) bool {
	if pp.name != "" {
		pp.name = fmt.Sprintf("%s.%s", pp.name, fd.Name())
	} else {
		pp.name = string(fd.Name())
	}

	if fd.Kind() == protoreflect.MessageKind {
		if fd.IsList() {
			for i := 0; i < val.List().Len(); i++ {
				val.List().Get(i).Message().Range(pp.countRecursive)
			}
			return true
		}
		// Does not support maps of messages; there aren't any in config.proto
		val.Message().Range(pp.countRecursive)
		return true
	}
	pp.count[pp.name]++
	return true
}

// Fields returns counts of all of the primitive fields in this configuration
//
// Field names in the map are qualified with where they are nested; this is not the same as a message's "FullName"
// Ex: A dashboard tab's name is "dashboards.dashboard_tab.name"
func Fields(cfg *configpb.Configuration) map[string]int64 {
	pp := protoPath{
		count: map[string]int64{},
	}

	cfg.ProtoReflect().Range(pp.countRecursive)
	return pp.count
}
