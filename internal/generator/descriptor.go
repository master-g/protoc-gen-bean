package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// Descriptor represents a protocol buffer message.
type Descriptor struct {
	common
	*descriptor.DescriptorProto
	parent   *Descriptor            // The containing message, if any.
	nested   []*Descriptor          // Inner messages, if any.
	enums    []*EnumDescriptor      // Inner enums, if any.
	ext      []*ExtensionDescriptor // Extensions, if any.
	typename []string               // Cached typename vector.
	index    int                    // The index into the container, whether the file or another message.
	path     string                 // The SourceCodeInfo path as comma-separated integers.
	group    bool
}

// TypeName returns the elements of the dotted type name.
// The package name is not part of this name.
func (d *Descriptor) TypeName() []string {
	if d.typename != nil {
		return d.typename
	}
	n := 0
	for parent := d; parent != nil; parent = parent.parent {
		n++
	}
	s := make([]string, n)
	for parent := d; parent != nil; parent = parent.parent {
		n--
		s[n] = parent.GetName()
	}
	d.typename = s
	return s
}

// EnumDescriptor describes an enum. If it's at top level, its parent will be nil.
// Otherwise it will be the descriptor of the message in which it is defined.
type EnumDescriptor struct {
	common
	*descriptor.EnumDescriptorProto
	parent   *Descriptor // The containing message, if any.
	typename []string    // Cached typename vector.
	index    int         // The index into the container, whether the file or a message.
	path     string      // The SourceCodeInfo path as comma-separated integers.
}

// TypeName returns the elements of the dotted type name.
// The package name is not part of this name.
func (e *EnumDescriptor) TypeName() (s []string) {
	if e.typename != nil {
		return e.typename
	}
	name := e.GetName()
	if e.parent == nil {
		s = make([]string, 1)
	} else {
		pname := e.parent.TypeName()
		s = make([]string, len(pname)+1)
		copy(s, pname)
	}
	s[len(s)-1] = name
	e.typename = s
	return s
}

// Everything but the last element of the full type name, CamelCased.
// The values of type Foo.Bar are call Foo_value1... not Foo_Bar_value1... .
func (e *EnumDescriptor) prefix() string {
	if e.parent == nil {
		// If the enum is not part of a message, the prefix is just the type name.
		return CamelCase(*e.Name) + "_"
	}
	typeName := e.TypeName()
	return CamelCaseSlice(typeName[0:len(typeName)-1]) + "_"
}

// The integer value of the named constant in this enumerated type.
func (e *EnumDescriptor) integerValueAsString(name string) string {
	for _, c := range e.Value {
		if c.GetName() == name {
			return fmt.Sprint(c.GetNumber())
		}
	}
	log.Fatal("cannot find value for enum constant")
	return ""
}

// ExtensionDescriptor describes an extension. If it's at top level, its parent will be nil.
// Otherwise it will be the descriptor of the message in which it is defined.
type ExtensionDescriptor struct {
	common
	*descriptor.FieldDescriptorProto
	parent *Descriptor // The containing message, if any.
}

// TypeName returns the elements of the dotted type name.
// The package name is not part of this name.
func (e *ExtensionDescriptor) TypeName() (s []string) {
	name := e.GetName()
	if e.parent == nil {
		// top-level extension
		s = make([]string, 1)
	} else {
		pname := e.parent.TypeName()
		s = make([]string, len(pname)+1)
		copy(s, pname)
	}
	s[len(s)-1] = name
	return s
}

// DescName returns the variable name used for the generated descriptor.
func (e *ExtensionDescriptor) DescName() string {
	// The full type name.
	typeName := e.TypeName()
	// Each scope of the extension is individually CamelCased, and all are joined with "_" with an "E_" prefix.
	for i, s := range typeName {
		typeName[i] = CamelCase(s)
	}
	return "E_" + strings.Join(typeName, "_")
}

// ImportedDescriptor describes a type that has been publicly imported from another file.
type ImportedDescriptor struct {
	common
	o Object
}

func (id *ImportedDescriptor) TypeName() []string { return id.o.TypeName() }

// FileDescriptor describes an protocol buffer descriptor file (.proto).
// It includes slices of all the messages and enums defined within it.
// Those slices are constructed by WrapTypes.
type FileDescriptor struct {
	*descriptor.FileDescriptorProto
	desc []*Descriptor          // All the messages defined in this file.
	enum []*EnumDescriptor      // All the enums defined in this file.
	ext  []*ExtensionDescriptor // All the top-level extensions defined in this file.
	imp  []*ImportedDescriptor  // All types defined in files publicly imported by this file.

	// Comments, stored as a map of path (comma-separated integers) to the comment.
	comments map[string]*descriptor.SourceCodeInfo_Location

	// The full list of symbols that are exported,
	// as a map from the exported object to its symbols.
	// This is used for supporting public imports.
	exported map[Object][]symbol

	importPath  GoImportPath  // Import path of this file's package.
	packageName GoPackageName // Name of this file's Go package.

	proto3 bool // whether to generate proto3 code for this file
}

// VarName is the variable name we'll use in the generated code to refer
// to the compressed bytes of this descriptor. It is not exported, so
// it is only valid inside the generated package.
func (d *FileDescriptor) VarName() string {
	h := sha256.Sum256([]byte(d.GetName()))
	return fmt.Sprintf("fileDescriptor_%s", hex.EncodeToString(h[:8]))
}

// goPackageOption interprets the file's go_package option.
// If there is no go_package, it returns ("", "", false).
// If there's a simple name, it returns ("", pkg, true).
// If the option implies an import path, it returns (impPath, pkg, true).
func (d *FileDescriptor) goPackageOption() (impPath GoImportPath, pkg GoPackageName, ok bool) {
	opt := d.GetOptions().GetGoPackage()
	if opt == "" {
		return "", "", false
	}
	// A semicolon-delimited suffix delimits the import path and package name.
	sc := strings.Index(opt, ";")
	if sc >= 0 {
		return GoImportPath(opt[:sc]), cleanPackageName(opt[sc+1:]), true
	}
	// The presence of a slash implies there's an import path.
	slash := strings.LastIndex(opt, "/")
	if slash >= 0 {
		return GoImportPath(opt), cleanPackageName(opt[slash+1:]), true
	}
	return "", cleanPackageName(opt), true
}

// goFileName returns the output name for the generated Go file.
func (d *FileDescriptor) goFileName(pathType pathType) string {
	name := *d.Name
	if ext := path.Ext(name); ext == ".proto" || ext == ".protodevel" {
		name = name[:len(name)-len(ext)]
	}
	name += ".pb.go"

	if pathType == pathTypeSourceRelative {
		return name
	}

	// Does the file have a "go_package" option?
	// If it does, it may override the filename.
	if impPath, _, ok := d.goPackageOption(); ok && impPath != "" {
		// Replace the existing dirname with the declared import path.
		_, name = path.Split(name)
		name = path.Join(string(impPath), name)
		return name
	}

	return name
}

func (d *FileDescriptor) addExport(obj Object, sym symbol) {
	d.exported[obj] = append(d.exported[obj], sym)
}
