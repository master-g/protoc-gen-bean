// Go support for Protocol Buffers - Google's data interchange format
//
// Copyright 2010 The Go Authors.  All rights reserved.
// https://github.com/golang/protobuf
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//     * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//     * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//     * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// https://github.com/golang/protobuf/blob/master/protoc-gen-go/generator/generator.go

/*
	The code generator for the plugin for the Google protocol buffer compiler.
	It generates Java code from the protocol buffer description files read by the
	main routine.
*/
package generator

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

// Each type we import as a protocol buffer (other than FileDescriptorProto) needs
// a pointer to the FileDescriptorProto that represents it.  These types achieve that
// wrapping by placing each Proto inside a struct with the pointer to its File. The
// structs have the same names as their contents, with "Proto" removed.
// FileDescriptor is used to store the things that it points to.

// The file and package name method are common to messages and enums.
type common struct {
	file *descriptor.FileDescriptorProto // File this object comes from.
}

// PackageName is name in the package clause in the generated file.
func (c *common) PackageName() string { return uniquePackageOf(c.file) }

func (c *common) File() *descriptor.FileDescriptorProto { return c.file }

func fileIsProto3(file *descriptor.FileDescriptorProto) bool {
	return file.GetSyntax() == "proto3"
}

func (c *common) proto3() bool { return fileIsProto3(c.file) }

func (c *common) BeanPackageName(g *Generator) string {
	if c.file != nil && c.file.GetOptions().GetJavaPackage() != "" {
		pkg := c.file.GetOptions().GetJavaPackage()
		sl := strings.Split(pkg, ".")
		sl = sl[:len(sl)-1]
		if g.VoPackage != "" {
			sl = append(sl, g.VoPackage)
		}
		protoSl := strings.Split(c.file.GetPackage(), ".")
		sl = append(sl, protoSl[len(protoSl)-1])
		return strings.Join(sl, ".")
	}
	return ""
}

func (c *common) BeanPath(g *Generator) string {
	pkgName := c.BeanPackageName(g)
	sl := strings.Split(pkgName, ".")
	return path.Join(sl...)
}

// Descriptor represents a protocol buffer message.
type Descriptor struct {
	common
	*descriptor.DescriptorProto
	parent   *Descriptor       // The containing message, if any.
	nested   []*Descriptor     // Inner messages, if any.
	enums    []*EnumDescriptor // Inner enums, if any.
	typename []string          // Cached typename vector.
	index    int               // The index into the container, whether the file or another message.
	path     string            // The SourceCodeInfo path as comma-separated integers.
	group    bool
}

func (d *Descriptor) BeanName() string {
	s := d.TypeName()
	return s[len(s)-1]
}

func (d *Descriptor) BeanFileName(g *Generator) string {
	name := *d.Name
	if ext := path.Ext(name); ext == ".proto" || ext == ".protodevel" {
		name = name[:len(name)-len(ext)]
	}
	name += ".java"

	if beanPath := d.BeanPath(g); beanPath != "" {
		return path.Join(beanPath, name)
	}

	return name
}

func (d *Descriptor) IsInner() bool {
	n := 0
	for parent := d; parent != nil; parent = parent.parent {
		n++
	}
	return n > 1
}

func (d *Descriptor) KeyAsField() string {
	return "." + d.file.GetPackage() + "." + d.BeanName()
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
	s := make([]string, n, n)
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

func (e *EnumDescriptor) BeanName() string {
	s := e.TypeName()
	return s[len(s)-1]
}

func (e *EnumDescriptor) BeanFileName(g *Generator) string {
	name := *e.Name
	if ext := path.Ext(name); ext == ".proto" || ext == ".protodevel" {
		name = name[:len(name)-len(ext)]
	}
	name += ".java"

	if beanPath := e.BeanPath(g); beanPath != "" {
		return path.Join(beanPath, name)
	}

	return name
}

func (e *EnumDescriptor) IsInner() bool {
	return e.parent != nil
}

func (e *EnumDescriptor) KeyAsField() string {
	return "." + e.file.GetPackage() + "." + e.BeanName()
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

// FileDescriptor describes an protocol buffer descriptor file (.proto).
// It includes slices of all the messages and enums defined within it.
// Those slices are constructed by WrapTypes.
type FileDescriptor struct {
	*descriptor.FileDescriptorProto
	desc []*Descriptor     // All the messages defined in this file.
	enum []*EnumDescriptor // All the enums defined in this file.
	imp  []string          // All packages imported by this file.

	// Comments, stored as a map of path (comma-separated integers) to the comment.
	comments map[string]*descriptor.SourceCodeInfo_Location

	// The full list of symbols that are exported,
	// as a map from the exported object to its symbols.
	// This is used for supporting public imports.
	exported map[Object][]symbol

	index int // The index of this file in the list of files to generate code for

	proto3 bool // whether to generate proto3 code for this file
}

// PackageName is the package name we'll use in the generated code to refer to this file.
func (d *FileDescriptor) PackageName() string { return uniquePackageOf(d.FileDescriptorProto) }

// VarName is the variable name we'll use in the generated code to refer
// to the compressed bytes of this descriptor. It is not exported, so
// it is only valid inside the generated package.
func (d *FileDescriptor) VarName() string { return fmt.Sprintf("fileDescriptor%d", d.index) }

// javaPackageOption interprets the file's java_package option.
// If there is no java_package, it returns ("", "", false).
// If there's a simple name, it returns ("", pkg, true).
// If the option implies an import path, it returns (impPath, pkg, true).
func (d *FileDescriptor) javaPackageOption() (impPath, pkg string, ok bool) {
	pkg = d.GetOptions().GetJavaPackage()
	if pkg == "" {
		return
	}
	ok = true
	// The presence of a dot implies there's an import path.
	dot := strings.LastIndex(pkg, ".")
	if dot < 0 {
		return
	}
	impPath = strings.Replace(pkg, ".", "/", -1)
	return
}

// javaPackageName returns the java package name to use in the
// generated java file.  The result explicit reports whether the name
// came from an option java_package statement.  If explicit is false,
// the name was derived from the protocol buffer's package statement
// or the input file name.
func (d *FileDescriptor) javaPackageName() (name string, explicit bool) {
	// Does the file have a "java_package" option?
	if _, pkg, ok := d.javaPackageOption(); ok {
		return pkg, true
	}

	// Does the file have a package clause?
	if pkg := d.GetPackage(); pkg != "" {
		return pkg, false
	}
	// Use the file base name.
	return baseName(d.GetName()), false
}

// converterPackageName returns the output name for the generated Go file.
func (d *FileDescriptor) converterPackageName(g *Generator) string {
	if d.GetOptions().GetJavaPackage() != "" {
		pkg := d.GetOptions().GetJavaPackage()
		sl := strings.Split(pkg, ".")
		sl = sl[:len(sl)-1]
		if g.ConverterPackage != "" {
			sl = append(sl, g.ConverterPackage)
		}
		return strings.Join(sl, ".")
	}

	return ""
}

func (d *FileDescriptor) pb2beanClassName() string {
	clsName := d.GetOptions().GetJavaOuterClassname()
	if strings.HasPrefix(clsName, "Pb") {
		clsName = clsName[2:]
		return clsName + "Pb2JavaBean"
	}
	return ""
}

func (d *FileDescriptor) bean2pbClassName() string {
	clsName := d.GetOptions().GetJavaOuterClassname()
	if strings.HasPrefix(clsName, "Pb") {
		clsName = clsName[2:]
		return clsName + "JavaBean2Pb"
	}
	return ""
}

func (d *FileDescriptor) converterFileName(g *Generator) (b2p, p2b string) {
	pkgName := d.converterPackageName(g)
	b2pCls := d.bean2pbClassName()
	p2bCls := d.pb2beanClassName()
	if pkgName != "" && b2pCls != "" && p2bCls != "" {
		sl := strings.Split(pkgName, ".")
		sl = append(sl, b2pCls+".java")
		b2pCls = path.Join(sl...)
		sl = sl[:len(sl)-1]
		sl = append(sl, p2bCls+".java")
		p2bCls = path.Join(sl...)
		return b2pCls, p2bCls
	}
	return "", ""
}

// symbol is an interface representing an exported Go symbol.
type symbol interface {
	// GenerateAlias should generate an appropriate alias
	// for the symbol from the named package.
	GenerateAlias(g *Generator, pkg string)
}

// Object is an interface abstracting the abilities shared by enums, messages, extensions and imported objects.
type Object interface {
	PackageName() string // The name we use in our output (a_b_c), possibly renamed for uniqueness.
	TypeName() []string
	File() *descriptor.FileDescriptorProto
}

// Each package name we generate must be unique. The package we're generating
// gets its own name but every other package must have a unique name that does
// not conflict in the code we generate.  These names are chosen globally (although
// they don't have to be, it simplifies things to do them globally).
func uniquePackageOf(fd *descriptor.FileDescriptorProto) string {
	s, ok := uniquePackageName[fd]
	if !ok {
		log.Fatal("internal error: no package name defined for " + fd.GetName())
	}
	return s
}

// Generator is the type whose methods generate the output, stored in the associated response structure.
type Generator struct {
	*bytes.Buffer

	Request  *plugin.CodeGeneratorRequest  // The input.
	Response *plugin.CodeGeneratorResponse // The output.

	Param map[string]string // Command-line parameters.

	packageName      string                     // What we're calling ourselves.
	allFiles         []*FileDescriptor          // All files in the tree
	allFilesByName   map[string]*FileDescriptor // All files by filename.
	genFiles         []*FileDescriptor          // Those files we will generate output for.
	file             *FileDescriptor            // The file we are compiling now.
	usedPackages     map[string]bool            // Names of packages used in current file.
	typeNameToObject map[string]Object          // Key is a fully-qualified name in input syntax.
	init             []string                   // Lines to emit in the init function.
	indent           string
	writeOutput      bool

	type2file        map[string]*FileDescriptor
	type2pkg         map[string]string
	VoPackage        string // java value object package
	ConverterPackage string
}

// New creates a new generator and allocates the request and response protobufs.
func New() *Generator {
	g := new(Generator)
	g.Buffer = new(bytes.Buffer)
	g.Request = new(plugin.CodeGeneratorRequest)
	g.Response = new(plugin.CodeGeneratorResponse)
	return g
}

// Error reports a problem, including an error, and exits the program.
func (g *Generator) Error(err error, msgs ...string) {
	s := strings.Join(msgs, " ") + ":" + err.Error()
	log.Print("protoc-gen-bean: error:", s)
	os.Exit(1)
}

// Fail reports a problem and exits the program.
func (g *Generator) Fail(msgs ...string) {
	s := strings.Join(msgs, " ")
	log.Print("protoc-gen-bean: error:", s)
	os.Exit(1)
}

// CommandLineParameters breaks the comma-separated list of key=value pairs
// in the parameter (a member of the request protobuf) into a key/value map.
// It then sets file name mappings defined by those entries.
func (g *Generator) CommandLineParameters(parameter string) {
	g.Param = make(map[string]string)
	for _, p := range strings.Split(parameter, ",") {
		if i := strings.Index(p, "="); i < 0 {
			g.Param[p] = ""
		} else {
			g.Param[p[0:i]] = p[i+1:]
		}
	}

	for k, v := range g.Param {
		switch k {
		case "vopackage":
			g.VoPackage = v
		case "cvtpackage":
			g.ConverterPackage = v
		}
	}
}

// DefaultPackageName returns the package name printed for the object.
// If its file is in a different package, it returns the package name we're using for this file, plus ".".
// Otherwise it returns the empty string.
func (g *Generator) DefaultPackageName(obj Object) string {
	pkg := obj.PackageName()
	if pkg == g.packageName {
		return ""
	}
	return pkg + "."
}

// For each input file, the unique package name to use, underscored.
var uniquePackageName = make(map[*descriptor.FileDescriptorProto]string)

// Package names already registered.  Key is the name from the .proto file;
// value is the name that appears in the generated code.
var pkgNamesInUse = make(map[string]bool)

// Create and remember a guaranteed unique package name for this file descriptor.
// Pkg is the candidate name.  If f is nil, it's a builtin package like "proto" and
// has no file descriptor.
func RegisterUniquePackageName(pkg string, f *FileDescriptor) string {
	// Convert bad runes to dots before finding a unique alias.
	pkg = strings.Map(badToDot, pkg)

	for i, orig := 1, pkg; pkgNamesInUse[pkg]; i++ {
		// It's a duplicate; must rename.
		pkg = orig + strconv.Itoa(i)
	}
	// Install it.
	pkgNamesInUse[pkg] = true
	if f != nil {
		uniquePackageName[f.FileDescriptorProto] = pkg
	}
	return pkg
}

// SetPackageNames sets the package name for this run.
// The package name must agree across all files being generated.
// It also defines unique package names for all imported files.
func (g *Generator) SetPackageNames() {
	// Register the name for this package.  It will be the first name
	// registered so is guaranteed to be unmodified.
	pkg, _ := g.genFiles[0].javaPackageName()

	g.packageName = RegisterUniquePackageName(pkg, g.genFiles[0])

AllFiles:
	for _, f := range g.allFiles {
		for _, genf := range g.genFiles {
			if f == genf {
				// In this package already.
				uniquePackageName[f.FileDescriptorProto] = g.packageName
				continue AllFiles
			}
		}
		// The file is a dependency, so we want to ignore its java_package option
		// because that is only relevant for its specific generated output.
		pkg := f.GetPackage()
		if pkg == "" {
			pkg = baseName(*f.Name)
		}
		RegisterUniquePackageName(pkg, f)
	}
}

// WrapTypes walks the incoming data, wrapping DescriptorProtos, EnumDescriptorProtos
// and FileDescriptorProtos into file-referenced objects within the Generator.
// It also creates the list of files to generate and so should be called before GenerateAllFiles.
func (g *Generator) WrapTypes() {
	g.allFiles = make([]*FileDescriptor, 0, len(g.Request.ProtoFile))
	g.allFilesByName = make(map[string]*FileDescriptor, len(g.allFiles))
	for _, f := range g.Request.ProtoFile {
		// We must wrap the descriptors before we wrap the enums
		descs := wrapDescriptors(f)
		g.buildNestedDescriptors(descs)
		enums := wrapEnumDescriptors(f, descs)
		g.buildNestedEnums(descs, enums)
		fd := &FileDescriptor{
			FileDescriptorProto: f,
			desc:                descs,
			enum:                enums,
			exported:            make(map[Object][]symbol),
			proto3:              fileIsProto3(f),
		}
		extractComments(fd)
		g.allFiles = append(g.allFiles, fd)
		g.allFilesByName[f.GetName()] = fd
	}
	for _, fd := range g.allFiles {
		fd.imp = wrapImported(fd.FileDescriptorProto, g)
	}

	g.genFiles = make([]*FileDescriptor, 0, len(g.Request.FileToGenerate))
	for _, fileName := range g.Request.FileToGenerate {
		fd := g.allFilesByName[fileName]
		if fd == nil {
			g.Fail("could not find file named", fileName)
		}
		fd.index = len(g.genFiles)
		g.genFiles = append(g.genFiles, fd)
	}
}

// Scan the descriptors in this file.  For each one, build the slice of nested descriptors
func (g *Generator) buildNestedDescriptors(descs []*Descriptor) {
	for _, desc := range descs {
		if len(desc.NestedType) != 0 {
			for _, nest := range descs {
				if nest.parent == desc {
					desc.nested = append(desc.nested, nest)
				}
			}
			if len(desc.nested) != len(desc.NestedType) {
				g.Fail("internal error: nesting failure for", desc.GetName())
			}
		}
	}
}

func (g *Generator) buildNestedEnums(descs []*Descriptor, enums []*EnumDescriptor) {
	for _, desc := range descs {
		if len(desc.EnumType) != 0 {
			for _, enum := range enums {
				if enum.parent == desc {
					desc.enums = append(desc.enums, enum)
				}
			}
			if len(desc.enums) != len(desc.EnumType) {
				g.Fail("internal error: enum nesting failure for", desc.GetName())
			}
		}
	}
}

// Construct the Descriptor
func newDescriptor(desc *descriptor.DescriptorProto, parent *Descriptor, file *descriptor.FileDescriptorProto, index int) *Descriptor {
	d := &Descriptor{
		common:          common{file},
		DescriptorProto: desc,
		parent:          parent,
		index:           index,
	}
	if parent == nil {
		d.path = fmt.Sprintf("%d,%d", messagePath, index)
	} else {
		d.path = fmt.Sprintf("%s,%d,%d", parent.path, messageMessagePath, index)
	}

	// The only way to distinguish a group from a message is whether
	// the containing message has a TYPE_GROUP field that matches.
	if parent != nil {
		parts := d.TypeName()
		if file.Package != nil {
			parts = append([]string{*file.Package}, parts...)
		}
		exp := "." + strings.Join(parts, ".")
		for _, field := range parent.Field {
			if field.GetType() == descriptor.FieldDescriptorProto_TYPE_GROUP && field.GetTypeName() == exp {
				d.group = true
				break
			}
		}
	}

	return d
}

// Return a slice of all the Descriptors defined within this file
func wrapDescriptors(file *descriptor.FileDescriptorProto) []*Descriptor {
	sl := make([]*Descriptor, 0, len(file.MessageType)+10)
	for i, desc := range file.MessageType {
		sl = wrapThisDescriptor(sl, desc, nil, file, i)
	}
	return sl
}

// Wrap this Descriptor, recursively
func wrapThisDescriptor(sl []*Descriptor, desc *descriptor.DescriptorProto, parent *Descriptor, file *descriptor.FileDescriptorProto, index int) []*Descriptor {
	sl = append(sl, newDescriptor(desc, parent, file, index))
	me := sl[len(sl)-1]
	for i, nested := range desc.NestedType {
		sl = wrapThisDescriptor(sl, nested, me, file, i)
	}
	return sl
}

// Construct the EnumDescriptor
func newEnumDescriptor(desc *descriptor.EnumDescriptorProto, parent *Descriptor, file *descriptor.FileDescriptorProto, index int) *EnumDescriptor {
	ed := &EnumDescriptor{
		common:              common{file},
		EnumDescriptorProto: desc,
		parent:              parent,
		index:               index,
	}
	if parent == nil {
		ed.path = fmt.Sprintf("%d,%d", enumPath, index)
	} else {
		ed.path = fmt.Sprintf("%s,%d,%d", parent.path, messageEnumPath, index)
	}
	return ed
}

// Return a slice of all the EnumDescriptors defined within this file
func wrapEnumDescriptors(file *descriptor.FileDescriptorProto, descs []*Descriptor) []*EnumDescriptor {
	sl := make([]*EnumDescriptor, 0, len(file.EnumType)+10)
	// Top-level enums.
	for i, enum := range file.EnumType {
		sl = append(sl, newEnumDescriptor(enum, nil, file, i))
	}
	// Enums within messages. Enums within embedded messages appear in the outer-most message.
	for _, nested := range descs {
		for i, enum := range nested.EnumType {
			sl = append(sl, newEnumDescriptor(enum, nested, file, i))
		}
	}
	return sl
}

// Return a slice of all the packages that are imported into this file.
func wrapImported(file *descriptor.FileDescriptorProto, g *Generator) (sl []string) {
	for _, index := range file.Dependency {
		df := g.fileByName(index)
		pkg := df.GetOptions().GetJavaPackage()
		if pkg != "" {
			sl = append(sl, pkg)
		}
	}
	return
}

func extractComments(file *FileDescriptor) {
	file.comments = make(map[string]*descriptor.SourceCodeInfo_Location)
	for _, loc := range file.GetSourceCodeInfo().GetLocation() {
		if loc.LeadingComments == nil {
			continue
		}
		var p []string
		for _, n := range loc.Path {
			p = append(p, strconv.Itoa(int(n)))
		}
		file.comments[strings.Join(p, ",")] = loc
	}
}

// BuildTypeNameMap builds the map from fully qualified type names to objects.
// The key names for the map come from the input data, which puts a period at the beginning.
// It should be called after SetPackageNames and before GenerateAllFiles.
func (g *Generator) BuildTypeNameMap() {
	g.typeNameToObject = make(map[string]Object)
	for _, f := range g.allFiles {
		// The names in this loop are defined by the proto world, not us, so the
		// package name may be empty.  If so, the dotted package name of X will
		// be ".X"; otherwise it will be ".pkg.X".
		dottedPkg := "." + f.GetPackage()
		if dottedPkg != "." {
			dottedPkg += "."
		}
		for _, enum := range f.enum {
			name := dottedPkg + dottedSlice(enum.TypeName())
			g.typeNameToObject[name] = enum
		}
		for _, desc := range f.desc {
			name := dottedPkg + dottedSlice(desc.TypeName())
			g.typeNameToObject[name] = desc
		}
	}
}

// ObjectNamed, given a fully-qualified input type name as it appears in the input data,
// returns the descriptor for the message or enum with that name.
func (g *Generator) ObjectNamed(typeName string) Object {
	o, ok := g.typeNameToObject[typeName]
	if !ok {
		g.Fail("can't find object with type", typeName)
	}

	// TODO

	return o
}

// P prints the arguments to the generated output.  It handles strings and int32s, plus
// handling indirections because they may be *string, etc.
func (g *Generator) P(str ...interface{}) {
	if !g.writeOutput {
		return
	}
	g.WriteString(g.indent)
	for _, v := range str {
		switch s := v.(type) {
		case string:
			g.WriteString(s)
		case *string:
			g.WriteString(*s)
		case bool:
			fmt.Fprintf(g, "%t", s)
		case *bool:
			fmt.Fprintf(g, "%t", *s)
		case int:
			fmt.Fprintf(g, "%d", s)
		case *int32:
			fmt.Fprintf(g, "%d", *s)
		case *int64:
			fmt.Fprintf(g, "%d", *s)
		case float64:
			fmt.Fprintf(g, "%g", s)
		case *float64:
			fmt.Fprintf(g, "%g", *s)
		default:
			g.Fail(fmt.Sprintf("unknown type in printer: %T", v))
		}
	}
	g.WriteByte('\n')
}

// addInitf stores the given statement to be printed inside the file's init function.
// The statement is given as a format specifier and arguments.
func (g *Generator) addInitf(stmt string, a ...interface{}) {
	g.init = append(g.init, fmt.Sprintf(stmt, a...))
}

const indent string = "    "

// In Indents the output one tab stop.
func (g *Generator) In() { g.indent += indent }

// Out unindents the output one tab stop.
func (g *Generator) Out() {
	if len(g.indent) > 0 {
		g.indent = g.indent[len(indent):]
	}
}

// Beans

func (g *Generator) generateBeanHeader(obj interface{}) {
	var pkgName string
	var fileName string
	if msg, ok := obj.(*Descriptor); ok {
		pkgName = msg.BeanPackageName(g)
		fileName = msg.file.GetName()
	} else if enum, ok := obj.(*EnumDescriptor); ok {
		pkgName = enum.BeanPackageName(g)
		fileName = enum.file.GetName()
	} else if fd, ok := obj.(*FileDescriptor); ok {
		pkgName = fd.converterPackageName(g)
		fileName = fd.GetName()
	}
	g.P("package ", pkgName, ";")
	g.P()
	g.P("// Code generated by protoc-gen-bean. DO NOT EDIT.")
	g.P("// ", time.Now().Format("2006-01-02 Mon 15:04:05 UTC-0700"))
	g.P("//")
	g.P("//     ", fileName)
	g.P("//")
	g.P()
	g.PrintComments(strconv.Itoa(packagePath))
}

func (g *Generator) compileEnum(enum *EnumDescriptor) {
	var defaultName string
	var defaultValue int32
	defaultKeys := []string{
		"unknown",
		"default",
		"invalid",
	}

	ename := enum.GetName()
	g.PrintComments(enum.path)
	g.P("public enum ", ename, " {")
	g.P()
	g.In()
	// search for default value
	e := enum.Value[0]
	for _, k := range defaultKeys {
		name := strings.ToLower(*e.Name)
		if strings.Contains(name, k) {
			defaultName = name
			defaultValue = *e.Number
			break
		}
	}
	if defaultName == "" {
		defaultName = "Unknown"
		defaultValue = *e.Number - 1
	}
	g.P(defaultName, "(", &defaultValue, "),")

	// process values
	for i, e := range enum.Value {
		name := *e.Name

		// value comment
		etorPath := fmt.Sprintf("%s,%d,%d", enum.path, enumValuePath, i)
		g.PrintComments(etorPath)

		tailComments := g.TailingComments(etorPath)

		if i != len(enum.Value)-1 {
			g.P(name, "(", e.Number, "),", tailComments)
		} else {
			g.P(name, "(", e.Number, ");", tailComments)
		}
	}
	g.P()
	g.P("public int code;")
	g.P()
	g.P(ename, "(int code) { this.code = code; }")
	g.P()
	g.P("public static ", ename, " valueOf(final int code) {")
	g.In()
	g.P("for (", ename, " c : ", ename, ".values()) {")
	g.In()
	g.P("if (code == c.code) return c;")
	g.Out()
	g.P("}")
	g.P("return ", defaultName, ";")
	g.Out()
	g.P("}")
	g.Out()
	g.P("}")
}

func (g *Generator) extractImports(msg *Descriptor, prj, sys map[string]bool) {
	fields := make([]*descriptor.FieldDescriptorProto, 0)
	for _, m := range msg.nested {
		for _, f := range m.Field {
			fields = append(fields, f)
		}
	}

	for _, f := range msg.Field {
		fields = append(fields, f)
	}

	for _, f := range fields {
		if isRepeated(f) && !sys["java.util.List"] {
			sys["java.util.List"] = true
		}
		k := f.GetTypeName()
		v, ok := g.type2pkg[k]
		if ok {
			prj[v] = true
		}
	}
}

func (g *Generator) compileImport(msg *Descriptor) {
	prjImports := make(map[string]bool)
	sysImports := make(map[string]bool)
	g.extractImports(msg, prjImports, sysImports)

	keys := []string{}
	for k, _ := range prjImports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g.P("import ", k, ";")
	}
	if len(prjImports) != 0 {
		g.P()
	}

	keys = []string{}
	for k, _ := range sysImports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g.P("import ", k, ";")
	}
	g.P()
}

func (g *Generator) compileMessage(msg *Descriptor) {
	g.PrintComments(msg.path)
	g.P("public class ", msg.BeanName(), " {")
	g.In() // >

	for i, f := range msg.Field {
		ftorPath := fmt.Sprintf("%s,%d,%d", msg.path, messageFieldPath, i)

		g.PrintComments(ftorPath)

		g.P("// loc:", ftorPath)
		if loc, ok := g.file.comments[ftorPath]; ok {
			text := strings.TrimSuffix(loc.GetTrailingComments(), "\n")
			g.P("// >>> ", text)
		}

		g.P("public ", g.JavaType(msg, f), " ", CamelCase(f.GetName()), ";", g.TailingComments(ftorPath))
	}
	if len(msg.Field) > 0 {
		g.P()
		g.P("@Override")
		g.P("public String toString() {")
		g.In() // >>
		g.P("return ", strconv.Quote(msg.BeanName()+"{"), " +")
		g.In() // >>>
		g.In() // >>>>
		for i, f := range msg.Field {
			varName := CamelCase(f.GetName())
			var tail string
			if i == len(msg.Field)-1 {
				tail = strconv.Quote("}") + ";"
			} else {
				tail = strconv.Quote(",") + " +"
			}
			if g.JavaType(msg, f) == "String" {
				g.P(strconv.Quote(varName+"='"), " + ", varName, " + '\\'' + ", tail)
			} else {
				g.P(strconv.Quote(varName+"="), " + ", varName, " + ", tail)
			}
		}
		g.Out() // >>>
		g.Out() // >>
		g.Out() // >
		g.P("}")
	}
	g.Out() //
}

func (g *Generator) compileMessages(msg *Descriptor) {
	g.compileImport(msg)

	g.compileMessage(msg)

	if len(msg.enums) != 0 {
		g.P()
		g.In()
		for _, e := range msg.enums {
			g.compileEnum(e)
		}
		g.Out()
	}

	if len(msg.nested) != 0 {
		g.P()
		g.In()
		for _, m := range msg.nested {
			g.compileMessage(m)
			g.P("}")
			g.P()
		}
		g.Out()
	}

	g.P("}")
}

func (g *Generator) compileBean(obj interface{}) string {
	g.Reset()
	g.writeOutput = true
	g.generateBeanHeader(obj)
	g.P()
	if enum, ok := obj.(*EnumDescriptor); ok {
		g.compileEnum(enum)
	} else if msg, ok := obj.(*Descriptor); ok {
		g.compileMessages(msg)
	}
	g.P("// end of file")
	g.P()
	return g.String()
}

func (g *Generator) buildType2PackageMap(f *FileDescriptor) {
	if g.type2pkg == nil {
		g.type2pkg = make(map[string]string)
	}
	if g.type2file == nil {
		g.type2file = make(map[string]*FileDescriptor)
	}
	for _, e := range f.enum {
		if e.IsInner() {
			continue
		}
		g.type2file[e.KeyAsField()] = f
		g.type2pkg[e.KeyAsField()] = fmt.Sprintf("%v.%v", e.BeanPackageName(g), e.GetName())
	}
	for _, m := range f.desc {
		if m.IsInner() {
			continue
		}
		g.type2file[m.KeyAsField()] = f
		g.type2pkg[m.KeyAsField()] = fmt.Sprintf("%v.%v", m.BeanPackageName(g), m.GetName())
	}
}

func (g *Generator) GenerateAllBeans() {
	for _, file := range g.allFiles {
		g.file = file
		g.buildType2PackageMap(file)
		for _, e := range file.enum {
			if !e.IsInner() {
				g.compileBean(e)
				g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
					Name:    proto.String(e.BeanFileName(g)),
					Content: proto.String(g.String()),
				})
			}
		}
		for _, m := range file.desc {
			if !m.IsInner() {
				g.compileBean(m)
				g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
					Name:    proto.String(m.BeanFileName(g)),
					Content: proto.String(g.String()),
				})
			}
		}
	}
}

func (g *Generator) generateConverterImport(file *FileDescriptor) {
	prjImports := make(map[string]bool)
	sysImports := make(map[string]bool)
	for _, d := range file.desc {
		g.extractImports(d, prjImports, sysImports)
	}

	if sysImports["java.util.List"] {
		sysImports["java.util.ArrayList"] = true
	}

	prjImports["com.google.protobuf.InvalidProtocolBufferException"] = true
	desc := make([]*Descriptor, 0)
	for _, d := range file.desc {
		for _, m := range d.nested {
			desc = append(desc, m)
		}
		desc = append(desc, d)
	}
	for _, m := range desc {
		// vo packages
		voPkg, ok := g.type2pkg[m.KeyAsField()]
		if ok {
			prjImports[voPkg] = true
		}
		// protoc gen java packages
		javaPkg := m.file.GetOptions().GetJavaPackage()
		javaClsName := m.file.GetOptions().GetJavaOuterClassname()
		if javaPkg != "" && javaClsName != "" {
			prjImports[javaPkg+"."+javaClsName] = true
		}
		for _, f := range m.Field {
			typ := f.GetTypeName()
			if typ == "" {
				continue
			}
			ref := g.type2file[typ]
			if ref == file {
				continue
			}
			javaPkg := ref.GetOptions().GetJavaPackage()
			javaClsName := ref.GetOptions().GetJavaOuterClassname()
			if javaPkg != "" && javaClsName != "" {
				prjImports[javaPkg+"."+javaClsName] = true
			}
		}
	}

	keys := []string{}
	for k, _ := range prjImports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g.P("import ", k, ";")
	}
	if len(prjImports) != 0 {
		g.P()
	}

	keys = []string{}
	for k, _ := range sysImports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		g.P("import ", k, ";")
	}
	g.P()
}

func (g *Generator) generateBean2Pb(file *FileDescriptor) {
	g.Reset()
	g.writeOutput = true
	g.generateBeanHeader(file)
	g.P()
}

func (g *Generator) generatePbMessage2Bean(file *FileDescriptor, d *Descriptor) {
	if d.IsInner() {
		return
	}
	g.P("public static ", d.BeanName(), " to", d.BeanName(), "(byte[] data) {")
	g.In()
	g.P("try {")
	g.In()
	pbType := file.GetOptions().GetJavaOuterClassname() + "." + d.BeanName()
	pbBeanName := "pb" + d.BeanName()
	g.P(pbType, " ", pbBeanName, " = ", pbType, ".parseFrom(data);")
	g.P()
	varName := LowerCaseInitial(d.BeanName())
	g.P(d.BeanName(), " ", varName, " = new ", d.BeanName(), "();")
	for _, f := range d.Field {
		prefix := varName + "." + CamelCase(f.GetName())
		if isMessage(f) {
			cvt := g.type2file[f.GetTypeName()].pb2beanClassName()
			memberName := strings.Title(CamelCase(f.GetName()))
			if isRepeated(f) {
				g.P(prefix, " = new ArrayList<>();")
				g.P("for (int i = 0; i < ", pbBeanName, ".get", memberName, "Count(); i++) {")
				g.In()
				ref := g.type2file[f.GetTypeName()]
				refPbType := ref.GetOptions().GetJavaOuterClassname() + "." + g.FieldTypeName(f)
				refVarName := LowerCaseInitial(g.FieldTypeName(f)) + "_"
				refCvt := ""
				if ref != file {
					refCvt = cvt + "."
				}
				g.P(refPbType, " ", refVarName, " = ", pbBeanName, ".get", memberName, "(i);")
				g.P(prefix, ".add(", refCvt, "to", g.FieldTypeName(f), "(", refVarName, ".toByteArray()));")
				g.Out()
				g.P("}")
			} else {
				pbVar := pbBeanName + ".get" + memberName + "().toByteArray()"
				g.P(prefix, " = ", cvt, ".to", memberName, "(", pbVar, ");")
			}
		} else {
			g.P(prefix, " = ", pbBeanName, ".get", strings.Title(CamelCase(f.GetName())), "();")
		}
	}

	g.P()
	g.P("return ", varName, ";")
	g.Out()
	g.P("} catch (InvalidProtocolBufferException e) {")
	g.In()
	g.P("e.printStackTrace();")
	g.Out()
	g.P("}")
	g.P()
	g.P("return null;")
	g.Out()
	g.P("}")
	g.P()
}

func (g *Generator) generatePb2Bean(file *FileDescriptor) {
	g.Reset()
	g.writeOutput = true
	g.generateBeanHeader(file)
	g.P()
	g.generateConverterImport(file)
	g.P("public class ", file.pb2beanClassName(), " {")
	g.In()

	for _, m := range file.desc {
		g.generatePbMessage2Bean(file, m)
	}

	g.Out()
	g.P("}")
	g.P()
}

func (g *Generator) GenerateAllConverters() {
	for _, file := range g.allFiles {
		if len(file.desc) == 0 {
			continue
		}
		b2pf, p2bf := file.converterFileName(g)
		if b2pf != "" && p2bf != "" {
			g.generateBean2Pb(file)
			g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
				Name:    proto.String(b2pf),
				Content: proto.String(g.String()),
			})
			g.generatePb2Bean(file)
			g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
				Name:    proto.String(p2bf),
				Content: proto.String(g.String()),
			})
		}
	}
}

// PrintComments prints any comments from the source .proto file.
// The path is a comma-separated list of integers.
// It returns an indication of whether any comments were printed.
// See descriptor.proto for its format.
func (g *Generator) PrintComments(path string) bool {
	if !g.writeOutput {
		return false
	}
	if loc, ok := g.file.comments[path]; ok {
		text := strings.TrimSuffix(loc.GetLeadingComments(), "\n")
		for _, line := range strings.Split(text, "\n") {
			g.P("// ", strings.TrimPrefix(line, " "))
		}
		return true
	}
	return false
}

func (g *Generator) TailingComments(path string) string {
	if !g.writeOutput {
		return ""
	}
	sb := &strings.Builder{}
	if loc, ok := g.file.comments[path]; ok {
		text := strings.TrimSuffix(loc.GetTrailingComments(), "\n")
		for _, line := range strings.Split(text, "\n") {
			sb.WriteString(" // ")
			sb.WriteString(strings.TrimPrefix(line, " "))
		}
	}
	return sb.String()
}

func (g *Generator) fileByName(filename string) *FileDescriptor {
	return g.allFilesByName[filename]
}

// TypeName is the printed name appropriate for an item. If the object is in the current file,
// TypeName drops the package name and underscores the rest.
// Otherwise the object is from another package; and the result is the underscored
// package name followed by the item name.
// The result always has an initial capital.
func (g *Generator) TypeName(obj Object) string {
	return g.DefaultPackageName(obj) + CamelCaseSlice(obj.TypeName())
}

// TypeNameWithPackage is like TypeName, but always includes the package
// name even if the object is in our own package.
func (g *Generator) TypeNameWithPackage(obj Object) string {
	return obj.PackageName() + CamelCaseSlice(obj.TypeName())
}

func (g *Generator) FieldTypeName(field *descriptor.FieldDescriptorProto) string {
	if field.GetTypeName() == "" {
		return ""
	}
	sl := strings.Split(field.GetTypeName(), ".")
	return sl[len(sl)-1]
}

// JavaType returns a string representing the type name, and the wire type
func (g *Generator) JavaType(message *Descriptor, field *descriptor.FieldDescriptorProto) (typ string) {
	// TODO: Options.
	switch *field.Type {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		typ = "double"
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		typ = "float"
	case descriptor.FieldDescriptorProto_TYPE_INT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_UINT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FIXED64:
		typ = "long"
	case descriptor.FieldDescriptorProto_TYPE_INT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_UINT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FIXED32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		typ = "int"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		typ = "boolean"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		typ = "String"
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		desc := g.ObjectNamed(field.GetTypeName())
		typ = g.TypeName(desc)
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		desc := g.ObjectNamed(field.GetTypeName())
		typ = g.TypeName(desc)
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		typ = "byte[]"
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		desc := g.ObjectNamed(field.GetTypeName())
		typ = g.TypeName(desc)
	default:
		g.Fail("unknown type for", field.GetName())
	}
	if isRepeated(field) {
		typ = fmt.Sprintf("List<%v>", typ)
	} else if message != nil && message.proto3() {
		return
	} else if field.OneofIndex != nil && message != nil {
		return
	}
	return
}

var escapeChars = [256]byte{
	'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', '\\': '\\', '"': '"', '\'': '\'', '?': '?',
}

// unescape reverses the "C" escaping that protoc does for default values of bytes fields.
// It is best effort in that it effectively ignores malformed input. Seemingly invalid escape
// sequences are conveyed, unmodified, into the decoded result.
func unescape(s string) string {
	// NB: Sadly, we can't use strconv.Unquote because protoc will escape both
	// single and double quotes, but strconv.Unquote only allows one or the
	// other (based on actual surrounding quotes of its input argument).

	var out []byte
	for len(s) > 0 {
		// regular character, or too short to be valid escape
		if s[0] != '\\' || len(s) < 2 {
			out = append(out, s[0])
			s = s[1:]
		} else if c := escapeChars[s[1]]; c != 0 {
			// escape sequence
			out = append(out, c)
			s = s[2:]
		} else if s[1] == 'x' || s[1] == 'X' {
			// hex escape, e.g. "\x80
			if len(s) < 4 {
				// too short to be valid
				out = append(out, s[:2]...)
				s = s[2:]
				continue
			}
			v, err := strconv.ParseUint(s[2:4], 16, 8)
			if err != nil {
				out = append(out, s[:4]...)
			} else {
				out = append(out, byte(v))
			}
			s = s[4:]
		} else if '0' <= s[1] && s[1] <= '7' {
			// octal escape, can vary from 1 to 3 octal digits; e.g., "\0" "\40" or "\164"
			// so consume up to 2 more bytes or up to end-of-string
			n := len(s[1:]) - len(strings.TrimLeft(s[1:], "01234567"))
			if n > 3 {
				n = 3
			}
			v, err := strconv.ParseUint(s[1:1+n], 8, 8)
			if err != nil {
				out = append(out, s[:1+n]...)
			} else {
				out = append(out, byte(v))
			}
			s = s[1+n:]
		} else {
			// bad escape, just propagate the slash as-is
			out = append(out, s[0])
			s = s[1:]
		}
	}

	return string(out)
}

// And now lots of helper functions.

// Is c an ASCII lower-case letter?
func isASCIILower(c byte) bool {
	return 'a' <= c && c <= 'z'
}

// Is c an ASCII digit?
func isASCIIDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// CamelCase returns the CamelCased name.
// If there is an interior underscore followed by a lower case letter,
// drop the underscore and convert the letter to upper case.
// There is a remote possibility of this rewrite causing a name collision,
// but it's so remote we're prepared to pretend it's nonexistent - since the
// C++ generator lowercases names, it's extremely unlikely to have two fields
// with different capitalizations.
// In short, _my_field_name_2 becomes XMyFieldName_2.
func CamelCase(s string) string {
	if s == "" {
		return ""
	}
	t := make([]byte, 0, 32)
	i := 0
	if s[0] == '_' {
		// Need a capital letter; drop the '_'.
		t = append(t, 'X')
		i++
	}
	// Invariant: if the next letter is lower case, it must be converted
	// to upper case.
	// That is, we process a word at a time, where words are marked by _ or
	// upper case letter. Digits are treated as words.
	for ; i < len(s); i++ {
		c := s[i]
		if c == '_' && i+1 < len(s) && isASCIILower(s[i+1]) {
			continue // Skip the underscore in s.
		}
		if isASCIIDigit(c) {
			t = append(t, c)
			continue
		}
		// Assume we have a letter now - if not, it's a bogus identifier.
		// The next word is a sequence of characters that must start upper case.
		if isASCIILower(c) && i != 0 {
			c ^= ' ' // Make it a capital letter.
		}
		t = append(t, c) // Guaranteed not lower case.
		// Accept lower case sequence that follows.
		for i+1 < len(s) && isASCIILower(s[i+1]) {
			i++
			t = append(t, s[i])
		}
	}
	return string(t)
}

// CamelCaseSlice is like CamelCase, but the argument is a slice of strings to
// be joined with ".".
func CamelCaseSlice(elem []string) string { return CamelCase(strings.Join(elem, ".")) }

func LowerCaseInitial(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[0:1]) + s[1:]
}

// dottedSlice turns a sliced name into a dotted name.
func dottedSlice(elem []string) string { return strings.Join(elem, ".") }

// Is this field optional?
func isOptional(field *descriptor.FieldDescriptorProto) bool {
	return field.Label != nil && *field.Label == descriptor.FieldDescriptorProto_LABEL_OPTIONAL
}

// Is this field required?
func isRequired(field *descriptor.FieldDescriptorProto) bool {
	return field.Label != nil && *field.Label == descriptor.FieldDescriptorProto_LABEL_REQUIRED
}

// Is this field repeated?
func isRepeated(field *descriptor.FieldDescriptorProto) bool {
	return field.Label != nil && *field.Label == descriptor.FieldDescriptorProto_LABEL_REPEATED
}

func isMessage(field *descriptor.FieldDescriptorProto) bool {
	if field.Type == nil {
		return false
	}
	if *field.Type == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
		return true
	}
	return false
}

// Is this field a scalar numeric type?
func isScalar(field *descriptor.FieldDescriptorProto) bool {
	if field.Type == nil {
		return false
	}
	switch *field.Type {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE,
		descriptor.FieldDescriptorProto_TYPE_FLOAT,
		descriptor.FieldDescriptorProto_TYPE_INT64,
		descriptor.FieldDescriptorProto_TYPE_UINT64,
		descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_FIXED32,
		descriptor.FieldDescriptorProto_TYPE_BOOL,
		descriptor.FieldDescriptorProto_TYPE_UINT32,
		descriptor.FieldDescriptorProto_TYPE_ENUM,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_SINT32,
		descriptor.FieldDescriptorProto_TYPE_SINT64:
		return true
	default:
		return false
	}
}

// badToDot is the mapping function used to generate Java names from package names,
// which can be dotted in the input .proto file.  It replaces non-identifier characters such as
// underscore or dash with dot.
func badToDot(r rune) rune {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' {
		return r
	}
	return '.'
}

// baseName returns the last path element of the name, with the last dotted suffix removed.
func baseName(name string) string {
	// First, find the last element
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	// Now drop the suffix
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[0:i]
	}
	return name
}

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
