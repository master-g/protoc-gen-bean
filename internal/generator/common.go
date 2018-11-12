package generator

import (
	"go/build"
	"strconv"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// generatedCodeVersion indicates a version of the generated code.
// It is incremented whenever an incompatibility between the generated code and
// proto package is introduced; the generated code references
// a constant, proto.ProtoPackageIsVersionN (where N is generatedCodeVersion).
const generatedCodeVersion = 2

// A JavaImportPath is the import path of a Java package. e.g., "com.google.genproto.protobuf".
type JavaImportPath string

func (p JavaImportPath) String() string { return strconv.Quote(string(p)) }

// A JavaPackageName is the name of a Java package. e.g., "com.google.protobuf".
type JavaPackageName string

// Each type we import as a protocol buffer (other than FileDescriptorProto) needs
// a pointer to the FileDescriptorProto that represents it.  These types achieve that
// wrapping by placing each Proto inside a struct with the pointer to its File. The
// structs have the same names as their contents, with "Proto" removed.
// FileDescriptor is used to store the things that it points to.

// The file and package name method are common to messages and enums.
type common struct {
	file *FileDescriptor // File this object comes from.
}

// JavaImportPath is the import path of the Go package containing the type.
func (c *common) JavaImportPath() JavaImportPath {
	return c.file.importPath
}

func (c *common) File() *FileDescriptor { return c.file }

func fileIsProto3(file *descriptor.FileDescriptorProto) bool {
	return file.GetSyntax() == "proto3"
}

func (c *common) proto3() bool { return fileIsProto3(c.file.FileDescriptorProto) }

// The SourceCodeInfo message describes the location of elements of a parsed
// .proto file by way of a "path", which is a sequence of integers that
// describe the route from a FileDescriptorProto to the relevant submessage.
// The path alternates between a field number of a repeated field, and an index
// into that repeated field. The constants below define the field numbers that
// are used.
//
// See descriptor.proto for more information about this.
const (
	// tag numbers in FileDescriptorProto
	packagePath = 2 // package
	messagePath = 4 // message_type
	enumPath    = 5 // enum_type
	// tag numbers in DescriptorProto
	messageFieldPath   = 2 // field
	messageMessagePath = 3 // nested_type
	messageEnumPath    = 4 // enum_type
	messageOneofPath   = 8 // oneof_decl
	// tag numbers in EnumDescriptorProto
	enumValuePath = 2 // value
)

var supportTypeAliases bool

func init() {
	for _, tag := range build.Default.ReleaseTags {
		if tag == "go1.9" {
			supportTypeAliases = true
			return
		}
	}
}
