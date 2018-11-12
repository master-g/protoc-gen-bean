package generator

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/master-g/protoc-gen-bean/internal/remap"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

// Generator is the type whose methods generate the output, stored in the associated response structure.
type Generator struct {
	*bytes.Buffer

	Request  *plugin.CodeGeneratorRequest  // The input.
	Response *plugin.CodeGeneratorResponse // The output.

	Param             map[string]string // Command-line parameters.
	PackageImportPath string            // Go import path of the package we're generating code for
	ImportPrefix      string            // String to prefix to imported package file names.
	ImportMap         map[string]string // Mapping from .proto file name to import path

	Pkg map[string]string // The names under which we import support packages

	outputImportPath JavaImportPath                     // Package we're generating code for.
	allFiles         []*FileDescriptor                  // All files in the tree
	allFilesByName   map[string]*FileDescriptor         // All files by filename.
	genFiles         []*FileDescriptor                  // Those files we will generate output for.
	file             *FileDescriptor                    // The file we are compiling now.
	packageNames     map[JavaImportPath]JavaPackageName // Imported package names in the current file.
	usedPackages     map[JavaImportPath]bool            // Packages used in current file.
	usedPackageNames map[JavaPackageName]bool           // Package names used in the current file.
	addedImports     map[JavaImportPath]bool            // Additional imports to emit.
	typeNameToObject map[string]Object                  // Key is a fully-qualified name in input syntax.
	init             []string                           // Lines to emit in the init function.
	indent           string
	pathType         pathType // How to generate output filenames.
	writeOutput      bool
	annotateCode     bool                                       // whether to store annotations
	annotations      []*descriptor.GeneratedCodeInfo_Annotation // annotations to store
}

type pathType int

const (
	pathTypeImport pathType = iota
	pathTypeSourceRelative
)

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
	log.Print("protoc-gen-go: error:", s)
	os.Exit(1)
}

// Fail reports a problem and exits the program.
func (g *Generator) Fail(msgs ...string) {
	s := strings.Join(msgs, " ")
	log.Print("protoc-gen-go: error:", s)
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

	g.ImportMap = make(map[string]string)
	pluginList := "none" // Default list of plugin names to enable (empty means all).
	for k, v := range g.Param {
		switch k {
		case "import_prefix":
			g.ImportPrefix = v
		case "import_path":
			g.PackageImportPath = v
		case "paths":
			switch v {
			case "import":
				g.pathType = pathTypeImport
			case "source_relative":
				g.pathType = pathTypeSourceRelative
			default:
				g.Fail(fmt.Sprintf(`Unknown path type %q: want "import" or "source_relative".`, v))
			}
		case "plugins":
			pluginList = v
		case "annotate_code":
			if v == "true" {
				g.annotateCode = true
			}
		default:
			if len(k) > 0 && k[0] == 'M' {
				g.ImportMap[k[1:]] = v
			}
		}
	}
	if pluginList != "" {
		// Amend the set of plugins.
		enabled := make(map[string]bool)
		for _, name := range strings.Split(pluginList, "+") {
			enabled[name] = true
		}
		var nplugins []Plugin
		for _, p := range plugins {
			if enabled[p.Name()] {
				nplugins = append(nplugins, p)
			}
		}
		plugins = nplugins
	}
}

// DefaultPackageName returns the package name printed for the object.
// If its file is in a different package, it returns the package name we're using for this file, plus ".".
// Otherwise it returns the empty string.
func (g *Generator) DefaultPackageName(obj Object) string {
	importPath := obj.JavaImportPath()
	if importPath == g.outputImportPath {
		return ""
	}
	return string(g.JavaPackageName(importPath)) + "."
}

// JavaPackageName returns the name used for a package.
func (g *Generator) JavaPackageName(importPath JavaImportPath) JavaPackageName {
	if name, ok := g.packageNames[importPath]; ok {
		return name
	}
	name := cleanPackageName(baseName(string(importPath)))
	for i, orig := 1, name; g.usedPackageNames[name] || isGoPredeclaredIdentifier[string(name)]; i++ {
		name = orig + JavaPackageName(strconv.Itoa(i))
	}
	g.packageNames[importPath] = name
	g.usedPackageNames[name] = true
	return name
}

// AddImport adds a package to the generated file's import section.
// It returns the name used for the package.
func (g *Generator) AddImport(importPath JavaImportPath) JavaPackageName {
	g.addedImports[importPath] = true
	return g.JavaPackageName(importPath)
}

var globalPackageNames = map[JavaPackageName]bool{
	"fmt":   true,
	"math":  true,
	"proto": true,
}

// Create and remember a guaranteed unique package name. Pkg is the candidate name.
// The FileDescriptor parameter is unused.
func RegisterUniquePackageName(pkg string, f *FileDescriptor) string {
	name := cleanPackageName(pkg)
	for i, orig := 1, name; globalPackageNames[name]; i++ {
		name = orig + JavaPackageName(strconv.Itoa(i))
	}
	globalPackageNames[name] = true
	return string(name)
}

var isGoKeyword = map[string]bool{
	"break":       true,
	"case":        true,
	"chan":        true,
	"const":       true,
	"continue":    true,
	"default":     true,
	"else":        true,
	"defer":       true,
	"fallthrough": true,
	"for":         true,
	"func":        true,
	"go":          true,
	"goto":        true,
	"if":          true,
	"import":      true,
	"interface":   true,
	"map":         true,
	"package":     true,
	"range":       true,
	"return":      true,
	"select":      true,
	"struct":      true,
	"switch":      true,
	"type":        true,
	"var":         true,
}

var isGoPredeclaredIdentifier = map[string]bool{
	"append":     true,
	"bool":       true,
	"byte":       true,
	"cap":        true,
	"close":      true,
	"complex":    true,
	"complex128": true,
	"complex64":  true,
	"copy":       true,
	"delete":     true,
	"error":      true,
	"false":      true,
	"float32":    true,
	"float64":    true,
	"imag":       true,
	"int":        true,
	"int16":      true,
	"int32":      true,
	"int64":      true,
	"int8":       true,
	"iota":       true,
	"len":        true,
	"make":       true,
	"new":        true,
	"nil":        true,
	"panic":      true,
	"print":      true,
	"println":    true,
	"real":       true,
	"recover":    true,
	"rune":       true,
	"string":     true,
	"true":       true,
	"uint":       true,
	"uint16":     true,
	"uint32":     true,
	"uint64":     true,
	"uint8":      true,
	"uintptr":    true,
}

// defaultGoPackage returns the package name to use,
// derived from the import path of the package we're building code for.
func (g *Generator) defaultGoPackage() JavaPackageName {
	p := g.PackageImportPath
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return cleanPackageName(p)
}

// SetPackageNames sets the package name for this run.
// The package name must agree across all files being generated.
// It also defines unique package names for all imported files.
func (g *Generator) SetPackageNames() {
	g.outputImportPath = g.genFiles[0].importPath

	defaultPackageNames := make(map[JavaImportPath]JavaPackageName)
	for _, f := range g.genFiles {
		if _, p, ok := f.javaPackageOption(); ok {
			defaultPackageNames[f.importPath] = p
		}
	}
	for _, f := range g.genFiles {
		if _, p, ok := f.javaPackageOption(); ok {
			// Source file: option go_package = "quux/bar";
			f.packageName = p
		} else if p, ok := defaultPackageNames[f.importPath]; ok {
			// A go_package option in another file in the same package.
			//
			// This is a poor choice in general, since every source file should
			// contain a go_package option. Supported mainly for historical
			// compatibility.
			f.packageName = p
		} else if p := g.defaultGoPackage(); p != "" {
			// Command-line: import_path=quux/bar.
			//
			// The import_path flag sets a package name for files which don't
			// contain a go_package option.
			f.packageName = p
		} else if p := f.GetPackage(); p != "" {
			// Source file: package quux.bar;
			f.packageName = cleanPackageName(p)
		} else {
			// Source filename.
			f.packageName = cleanPackageName(baseName(f.GetName()))
		}
	}

	// Check that all files have a consistent package name and import path.
	for _, f := range g.genFiles[1:] {
		if a, b := g.genFiles[0].importPath, f.importPath; a != b {
			g.Fail(fmt.Sprintf("inconsistent package import paths: %v, %v", a, b))
		}
		if a, b := g.genFiles[0].packageName, f.packageName; a != b {
			g.Fail(fmt.Sprintf("inconsistent package names: %v, %v", a, b))
		}
	}

	// Names of support packages. These never vary (if there are conflicts,
	// we rename the conflicting package), so this could be removed someday.
	g.Pkg = map[string]string{
		"fmt":   "fmt",
		"math":  "math",
		"proto": "proto",
	}
}

// WrapTypes walks the incoming data, wrapping DescriptorProtos, EnumDescriptorProtos
// and FileDescriptorProtos into file-referenced objects within the Generator.
// It also creates the list of files to generate and so should be called before GenerateAllFiles.
func (g *Generator) WrapTypes() {
	g.allFiles = make([]*FileDescriptor, 0, len(g.Request.ProtoFile))
	g.allFilesByName = make(map[string]*FileDescriptor, len(g.allFiles))
	genFileNames := make(map[string]bool)
	for _, n := range g.Request.FileToGenerate {
		genFileNames[n] = true
	}
	for _, f := range g.Request.ProtoFile {
		fd := &FileDescriptor{
			FileDescriptorProto: f,
			exported:            make(map[Object][]symbol),
			proto3:              fileIsProto3(f),
		}
		// The import path may be set in a number of ways.
		if substitution, ok := g.ImportMap[f.GetName()]; ok {
			// Command-line: M=foo.proto=quux/bar.
			//
			// Explicit mapping of source file to import path.
			fd.importPath = JavaImportPath(substitution)
		} else if genFileNames[f.GetName()] && g.PackageImportPath != "" {
			// Command-line: import_path=quux/bar.
			//
			// The import_path flag sets the import path for every file that
			// we generate code for.
			fd.importPath = JavaImportPath(g.PackageImportPath)
		} else if p, _, _ := fd.javaPackageOption(); p != "" {
			// Source file: option go_package = "quux/bar";
			//
			// The go_package option sets the import path. Most users should use this.
			fd.importPath = p
		} else {
			// Source filename.
			//
			// Last resort when nothing else is available.
			fd.importPath = JavaImportPath(path.Dir(f.GetName()))
		}
		// We must wrap the descriptors before we wrap the enums
		fd.desc = wrapDescriptors(fd)
		g.buildNestedDescriptors(fd.desc)
		fd.enum = wrapEnumDescriptors(fd, fd.desc)
		g.buildNestedEnums(fd.desc, fd.enum)
		fd.ext = wrapExtensions(fd)
		extractComments(fd)
		g.allFiles = append(g.allFiles, fd)
		g.allFilesByName[f.GetName()] = fd
	}
	for _, fd := range g.allFiles {
		fd.imp = wrapImported(fd, g)
	}

	g.genFiles = make([]*FileDescriptor, 0, len(g.Request.FileToGenerate))
	for _, fileName := range g.Request.FileToGenerate {
		fd := g.allFilesByName[fileName]
		if fd == nil {
			g.Fail("could not find file named", fileName)
		}
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
func newDescriptor(desc *descriptor.DescriptorProto, parent *Descriptor, file *FileDescriptor, index int) *Descriptor {
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

	for _, field := range desc.Extension {
		d.ext = append(d.ext, &ExtensionDescriptor{common{file}, field, d})
	}

	return d
}

// Return a slice of all the Descriptors defined within this file
func wrapDescriptors(file *FileDescriptor) []*Descriptor {
	sl := make([]*Descriptor, 0, len(file.MessageType)+10)
	for i, desc := range file.MessageType {
		sl = wrapThisDescriptor(sl, desc, nil, file, i)
	}
	return sl
}

// Wrap this Descriptor, recursively
func wrapThisDescriptor(sl []*Descriptor, desc *descriptor.DescriptorProto, parent *Descriptor, file *FileDescriptor, index int) []*Descriptor {
	sl = append(sl, newDescriptor(desc, parent, file, index))
	me := sl[len(sl)-1]
	for i, nested := range desc.NestedType {
		sl = wrapThisDescriptor(sl, nested, me, file, i)
	}
	return sl
}

// Construct the EnumDescriptor
func newEnumDescriptor(desc *descriptor.EnumDescriptorProto, parent *Descriptor, file *FileDescriptor, index int) *EnumDescriptor {
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
func wrapEnumDescriptors(file *FileDescriptor, descs []*Descriptor) []*EnumDescriptor {
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

// Return a slice of all the top-level ExtensionDescriptors defined within this file.
func wrapExtensions(file *FileDescriptor) []*ExtensionDescriptor {
	var sl []*ExtensionDescriptor
	for _, field := range file.Extension {
		sl = append(sl, &ExtensionDescriptor{common{file}, field, nil})
	}
	return sl
}

// Return a slice of all the types that are publicly imported into this file.
func wrapImported(file *FileDescriptor, g *Generator) (sl []*ImportedDescriptor) {
	for _, index := range file.PublicDependency {
		df := g.fileByName(file.Dependency[index])
		for _, d := range df.desc {
			if d.GetOptions().GetMapEntry() {
				continue
			}
			sl = append(sl, &ImportedDescriptor{common{file}, d})
		}
		for _, e := range df.enum {
			sl = append(sl, &ImportedDescriptor{common{file}, e})
		}
		for _, ext := range df.ext {
			sl = append(sl, &ImportedDescriptor{common{file}, ext})
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
	return o
}

// AnnotatedAtoms is a list of atoms (as consumed by P) that records the file name and proto AST path from which they originated.
type AnnotatedAtoms struct {
	source string
	path   string
	atoms  []interface{}
}

// Annotate records the file name and proto AST path of a list of atoms
// so that a later call to P can emit a link from each atom to its origin.
func Annotate(file *FileDescriptor, path string, atoms ...interface{}) *AnnotatedAtoms {
	return &AnnotatedAtoms{source: *file.Name, path: path, atoms: atoms}
}

// printAtom prints the (atomic, non-annotation) argument to the generated output.
func (g *Generator) printAtom(v interface{}) {
	switch v := v.(type) {
	case string:
		g.WriteString(v)
	case *string:
		g.WriteString(*v)
	case bool:
		fmt.Fprint(g, v)
	case *bool:
		fmt.Fprint(g, *v)
	case int:
		fmt.Fprint(g, v)
	case *int32:
		fmt.Fprint(g, *v)
	case *int64:
		fmt.Fprint(g, *v)
	case float64:
		fmt.Fprint(g, v)
	case *float64:
		fmt.Fprint(g, *v)
	case JavaPackageName:
		g.WriteString(string(v))
	case JavaImportPath:
		g.WriteString(strconv.Quote(string(v)))
	default:
		g.Fail(fmt.Sprintf("unknown type in printer: %T", v))
	}
}

// P prints the arguments to the generated output.  It handles strings and int32s, plus
// handling indirections because they may be *string, etc.  Any inputs of type AnnotatedAtoms may emit
// annotations in a .meta file in addition to outputting the atoms themselves (if g.annotateCode
// is true).
func (g *Generator) P(str ...interface{}) {
	if !g.writeOutput {
		return
	}
	g.WriteString(g.indent)
	for _, v := range str {
		switch v := v.(type) {
		case *AnnotatedAtoms:
			begin := int32(g.Len())
			for _, v := range v.atoms {
				g.printAtom(v)
			}
			if g.annotateCode {
				end := int32(g.Len())
				var path []int32
				for _, token := range strings.Split(v.path, ",") {
					val, err := strconv.ParseInt(token, 10, 32)
					if err != nil {
						g.Fail("could not parse proto AST path: ", err.Error())
					}
					path = append(path, int32(val))
				}
				g.annotations = append(g.annotations, &descriptor.GeneratedCodeInfo_Annotation{
					Path:       path,
					SourceFile: &v.source,
					Begin:      &begin,
					End:        &end,
				})
			}
		default:
			g.printAtom(v)
		}
	}
	g.WriteByte('\n')
}

// addInitf stores the given statement to be printed inside the file's init function.
// The statement is given as a format specifier and arguments.
func (g *Generator) addInitf(stmt string, a ...interface{}) {
	g.init = append(g.init, fmt.Sprintf(stmt, a...))
}

// In Indents the output one tab stop.
func (g *Generator) In() { g.indent += "\t" }

// Out unindents the output one tab stop.
func (g *Generator) Out() {
	if len(g.indent) > 0 {
		g.indent = g.indent[1:]
	}
}

// GenerateAllFiles generates the output for all the files we're outputting.
func (g *Generator) GenerateAllFiles() {
	// Initialize the plugins
	for _, p := range plugins {
		p.Init(g)
	}
	// Generate the output. The generator runs for every file, even the files
	// that we don't generate output for, so that we can collate the full list
	// of exported symbols to support public imports.
	genFileMap := make(map[*FileDescriptor]bool, len(g.genFiles))
	for _, file := range g.genFiles {
		genFileMap[file] = true
	}
	for _, file := range g.allFiles {
		g.Reset()
		g.annotations = nil
		g.writeOutput = genFileMap[file]
		g.generate(file)
		if !g.writeOutput {
			continue
		}
		fname := file.javaFileName(g.pathType)
		g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(fname),
			Content: proto.String(g.String()),
		})
		if g.annotateCode {
			// Store the generated code annotations in text, as the protoc plugin protocol requires that
			// strings contain valid UTF-8.
			g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
				Name:    proto.String(file.javaFileName(g.pathType) + ".meta"),
				Content: proto.String(proto.CompactTextString(&descriptor.GeneratedCodeInfo{Annotation: g.annotations})),
			})
		}
	}
}

// Run all the plugins associated with the file.
func (g *Generator) runPlugins(file *FileDescriptor) {
	for _, p := range plugins {
		p.Generate(file)
	}
}

// Fill the response protocol buffer with the generated output for all the files we're
// supposed to generate.
func (g *Generator) generate(file *FileDescriptor) {
	g.file = file
	g.usedPackages = make(map[JavaImportPath]bool)
	g.packageNames = make(map[JavaImportPath]JavaPackageName)
	g.usedPackageNames = make(map[JavaPackageName]bool)
	g.addedImports = make(map[JavaImportPath]bool)
	for name := range globalPackageNames {
		g.usedPackageNames[name] = true
	}

	g.P("// This is a compile-time assertion to ensure that this generated file")
	g.P("// is compatible with the proto package it is being compiled against.")
	g.P("// A compilation error at this line likely means your copy of the")
	g.P("// proto package needs to be updated.")
	g.P("const _ = ", g.Pkg["proto"], ".ProtoPackageIsVersion", generatedCodeVersion, " // please upgrade the proto package")
	g.P()

	for _, td := range g.file.imp {
		g.generateImported(td)
	}
	for _, enum := range g.file.enum {
		g.generateEnum(enum)
	}
	for _, desc := range g.file.desc {
		// Don't generate virtual messages for maps.
		if desc.GetOptions().GetMapEntry() {
			continue
		}
		g.generateMessage(desc)
	}
	for _, ext := range g.file.ext {
		g.generateExtension(ext)
	}
	g.generateInitFunction()
	g.generateFileDescriptor(file)

	// Run the plugins before the imports so we know which imports are necessary.
	g.runPlugins(file)

	// Generate header and imports last, though they appear first in the output.
	rem := g.Buffer
	remAnno := g.annotations
	g.Buffer = new(bytes.Buffer)
	g.annotations = nil
	g.generateHeader()
	g.generateImports()
	if !g.writeOutput {
		return
	}
	// Adjust the offsets for annotations displaced by the header and imports.
	for _, anno := range remAnno {
		*anno.Begin += int32(g.Len())
		*anno.End += int32(g.Len())
		g.annotations = append(g.annotations, anno)
	}
	g.Write(rem.Bytes())

	// Reformat generated code and patch annotation locations.
	fset := token.NewFileSet()
	original := g.Bytes()
	if g.annotateCode {
		// make a copy independent of g; we'll need it after Reset.
		original = append([]byte(nil), original...)
	}
	fileAST, err := parser.ParseFile(fset, "", original, parser.ParseComments)
	if err != nil {
		// Print out the bad code with line numbers.
		// This should never happen in practice, but it can while changing generated code,
		// so consider this a debugging aid.
		var src bytes.Buffer
		s := bufio.NewScanner(bytes.NewReader(original))
		for line := 1; s.Scan(); line++ {
			fmt.Fprintf(&src, "%5d\t%s\n", line, s.Bytes())
		}
		g.Fail("bad Go source code was generated:", err.Error(), "\n"+src.String())
	}
	ast.SortImports(fset, fileAST)
	g.Reset()
	err = (&printer.Config{Mode: printer.TabIndent | printer.UseSpaces, Tabwidth: 8}).Fprint(g, fset, fileAST)
	if err != nil {
		g.Fail("generated Go source code could not be reformatted:", err.Error())
	}
	if g.annotateCode {
		m, err := remap.Compute(original, g.Bytes())
		if err != nil {
			g.Fail("formatted generated Go source code could not be mapped back to the original code:", err.Error())
		}
		for _, anno := range g.annotations {
			new, ok := m.Find(int(*anno.Begin), int(*anno.End))
			if !ok {
				g.Fail("span in formatted generated Go source code could not be mapped back to the original code")
			}
			*anno.Begin = int32(new.Pos)
			*anno.End = int32(new.End)
		}
	}
}

// Generate the header, including package definition
func (g *Generator) generateHeader() {
	g.P("// Code generated by protoc-gen-go. DO NOT EDIT.")
	if g.file.GetOptions().GetDeprecated() {
		g.P("// ", g.file.Name, " is a deprecated file.")
	} else {
		g.P("// source: ", g.file.Name)
	}
	g.P()
	g.PrintComments(strconv.Itoa(packagePath))
	g.P()
	g.P("package ", g.file.packageName)
	g.P()
}

// deprecationComment is the standard comment added to deprecated
// messages, fields, enums, and enum values.
var deprecationComment = "// Deprecated: Do not use."

// PrintComments prints any comments from the source .proto file.
// The path is a comma-separated list of integers.
// It returns an indication of whether any comments were printed.
// See descriptor.proto for its format.
func (g *Generator) PrintComments(path string) bool {
	if !g.writeOutput {
		return false
	}
	if c, ok := g.makeComments(path); ok {
		g.P(c)
		return true
	}
	return false
}

// makeComments generates the comment string for the field, no "\n" at the end
func (g *Generator) makeComments(path string) (string, bool) {
	loc, ok := g.file.comments[path]
	if !ok {
		return "", false
	}
	w := new(bytes.Buffer)
	nl := ""
	for _, line := range strings.Split(strings.TrimSuffix(loc.GetLeadingComments(), "\n"), "\n") {
		fmt.Fprintf(w, "%s//%s", nl, line)
		nl = "\n"
	}
	return w.String(), true
}

func (g *Generator) fileByName(filename string) *FileDescriptor {
	return g.allFilesByName[filename]
}

// weak returns whether the ith import of the current file is a weak import.
func (g *Generator) weak(i int32) bool {
	for _, j := range g.file.WeakDependency {
		if j == i {
			return true
		}
	}
	return false
}

// Generate the imports
func (g *Generator) generateImports() {
	imports := make(map[JavaImportPath]JavaPackageName)
	for i, s := range g.file.Dependency {
		fd := g.fileByName(s)
		importPath := fd.importPath
		// Do not import our own package.
		if importPath == g.file.importPath {
			continue
		}
		// Do not import weak imports.
		if g.weak(int32(i)) {
			continue
		}
		// Do not import a package twice.
		if _, ok := imports[importPath]; ok {
			continue
		}
		// We need to import all the dependencies, even if we don't reference them,
		// because other code and tools depend on having the full transitive closure
		// of protocol buffer types in the binary.
		packageName := g.JavaPackageName(importPath)
		if _, ok := g.usedPackages[importPath]; !ok {
			packageName = "_"
		}
		imports[importPath] = packageName
	}
	for importPath := range g.addedImports {
		imports[importPath] = g.JavaPackageName(importPath)
	}
	// We almost always need a proto import.  Rather than computing when we
	// do, which is tricky when there's a plugin, just import it and
	// reference it later. The same argument applies to the fmt and math packages.
	g.P("import (")
	g.P(g.Pkg["fmt"] + ` "fmt"`)
	g.P(g.Pkg["math"] + ` "math"`)
	g.P(g.Pkg["proto"]+" ", JavaImportPath(g.ImportPrefix)+"github.com/golang/protobuf/proto")
	for importPath, packageName := range imports {
		g.P(packageName, " ", JavaImportPath(g.ImportPrefix)+importPath)
	}
	g.P(")")
	g.P()
	// TODO: may need to worry about uniqueness across plugins
	for _, p := range plugins {
		p.GenerateImports(g.file)
		g.P()
	}
	g.P("// Reference imports to suppress errors if they are not otherwise used.")
	g.P("var _ = ", g.Pkg["proto"], ".Marshal")
	g.P("var _ = ", g.Pkg["fmt"], ".Errorf")
	g.P("var _ = ", g.Pkg["math"], ".Inf")
	g.P()
}

func (g *Generator) generateImported(id *ImportedDescriptor) {
	df := id.o.File()
	filename := *df.Name
	if df.importPath == g.file.importPath {
		// Don't generate type aliases for files in the same Go package as this one.
		return
	}
	if !supportTypeAliases {
		g.Fail(fmt.Sprintf("%s: public imports require at least go1.9", filename))
	}
	g.usedPackages[df.importPath] = true

	for _, sym := range df.exported[id.o] {
		sym.GenerateAlias(g, filename, g.JavaPackageName(df.importPath))
	}

	g.P()
}

// Generate the enum definitions for this EnumDescriptor.
func (g *Generator) generateEnum(enum *EnumDescriptor) {
	// The full type name
	typeName := enum.TypeName()
	// The full type name, CamelCased.
	ccTypeName := CamelCaseSlice(typeName)
	ccPrefix := enum.prefix()

	deprecatedEnum := ""
	if enum.GetOptions().GetDeprecated() {
		deprecatedEnum = deprecationComment
	}
	g.PrintComments(enum.path)
	g.P("type ", Annotate(enum.file, enum.path, ccTypeName), " int32", deprecatedEnum)
	g.file.addExport(enum, enumSymbol{ccTypeName, enum.proto3()})
	g.P("const (")
	for i, e := range enum.Value {
		etorPath := fmt.Sprintf("%s,%d,%d", enum.path, enumValuePath, i)
		g.PrintComments(etorPath)

		deprecatedValue := ""
		if e.GetOptions().GetDeprecated() {
			deprecatedValue = deprecationComment
		}

		name := ccPrefix + *e.Name
		g.P(Annotate(enum.file, etorPath, name), " ", ccTypeName, " = ", e.Number, " ", deprecatedValue)
		g.file.addExport(enum, constOrVarSymbol{name, "const", ccTypeName})
	}
	g.P(")")
	g.P()
	g.P("var ", ccTypeName, "_name = map[int32]string{")
	generated := make(map[int32]bool) // avoid duplicate values
	for _, e := range enum.Value {
		duplicate := ""
		if _, present := generated[*e.Number]; present {
			duplicate = "// Duplicate value: "
		}
		g.P(duplicate, e.Number, ": ", strconv.Quote(*e.Name), ",")
		generated[*e.Number] = true
	}
	g.P("}")
	g.P()
	g.P("var ", ccTypeName, "_value = map[string]int32{")
	for _, e := range enum.Value {
		g.P(strconv.Quote(*e.Name), ": ", e.Number, ",")
	}
	g.P("}")
	g.P()

	if !enum.proto3() {
		g.P("func (x ", ccTypeName, ") Enum() *", ccTypeName, " {")
		g.P("p := new(", ccTypeName, ")")
		g.P("*p = x")
		g.P("return p")
		g.P("}")
		g.P()
	}

	g.P("func (x ", ccTypeName, ") String() string {")
	g.P("return ", g.Pkg["proto"], ".EnumName(", ccTypeName, "_name, int32(x))")
	g.P("}")
	g.P()

	if !enum.proto3() {
		g.P("func (x *", ccTypeName, ") UnmarshalJSON(data []byte) error {")
		g.P("value, err := ", g.Pkg["proto"], ".UnmarshalJSONEnum(", ccTypeName, `_value, data, "`, ccTypeName, `")`)
		g.P("if err != nil {")
		g.P("return err")
		g.P("}")
		g.P("*x = ", ccTypeName, "(value)")
		g.P("return nil")
		g.P("}")
		g.P()
	}

	var indexes []string
	for m := enum.parent; m != nil; m = m.parent {
		// XXX: skip groups?
		indexes = append([]string{strconv.Itoa(m.index)}, indexes...)
	}
	indexes = append(indexes, strconv.Itoa(enum.index))
	g.P("func (", ccTypeName, ") EnumDescriptor() ([]byte, []int) {")
	g.P("return ", g.file.VarName(), ", []int{", strings.Join(indexes, ", "), "}")
	g.P("}")
	g.P()
	if enum.file.GetPackage() == "google.protobuf" && enum.GetName() == "NullValue" {
		g.P("func (", ccTypeName, `) XXX_WellKnownType() string { return "`, enum.GetName(), `" }`)
		g.P()
	}

	g.generateEnumRegistration(enum)
}

// The tag is a string like "varint,2,opt,name=fieldname,def=7" that
// identifies details of the field for the protocol buffer marshaling and unmarshaling
// code.  The fields are:
//	wire encoding
//	protocol tag number
//	opt,req,rep for optional, required, or repeated
//	packed whether the encoding is "packed" (optional; repeated primitives only)
//	name= the original declared name
//	enum= the name of the enum type if it is an enum-typed field.
//	proto3 if this field is in a proto3 message
//	def= string representation of the default value, if any.
// The default value must be in a representation that can be used at run-time
// to generate the default value. Thus bools become 0 and 1, for instance.
func (g *Generator) goTag(message *Descriptor, field *descriptor.FieldDescriptorProto, wiretype string) string {
	optrepreq := ""
	switch {
	case isOptional(field):
		optrepreq = "opt"
	case isRequired(field):
		optrepreq = "req"
	case isRepeated(field):
		optrepreq = "rep"
	}
	var defaultValue string
	if dv := field.DefaultValue; dv != nil { // set means an explicit default
		defaultValue = *dv
		// Some types need tweaking.
		switch *field.Type {
		case descriptor.FieldDescriptorProto_TYPE_BOOL:
			if defaultValue == "true" {
				defaultValue = "1"
			} else {
				defaultValue = "0"
			}
		case descriptor.FieldDescriptorProto_TYPE_STRING,
			descriptor.FieldDescriptorProto_TYPE_BYTES:
			// Nothing to do. Quoting is done for the whole tag.
		case descriptor.FieldDescriptorProto_TYPE_ENUM:
			// For enums we need to provide the integer constant.
			obj := g.ObjectNamed(field.GetTypeName())
			if id, ok := obj.(*ImportedDescriptor); ok {
				// It is an enum that was publicly imported.
				// We need the underlying type.
				obj = id.o
			}
			enum, ok := obj.(*EnumDescriptor)
			if !ok {
				log.Printf("obj is a %T", obj)
				if id, ok := obj.(*ImportedDescriptor); ok {
					log.Printf("id.o is a %T", id.o)
				}
				g.Fail("unknown enum type", CamelCaseSlice(obj.TypeName()))
			}
			defaultValue = enum.integerValueAsString(defaultValue)
		case descriptor.FieldDescriptorProto_TYPE_FLOAT:
			if def := defaultValue; def != "inf" && def != "-inf" && def != "nan" {
				if f, err := strconv.ParseFloat(defaultValue, 32); err == nil {
					defaultValue = fmt.Sprint(float32(f))
				}
			}
		case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
			if def := defaultValue; def != "inf" && def != "-inf" && def != "nan" {
				if f, err := strconv.ParseFloat(defaultValue, 64); err == nil {
					defaultValue = fmt.Sprint(f)
				}
			}
		}
		defaultValue = ",def=" + defaultValue
	}
	enum := ""
	if *field.Type == descriptor.FieldDescriptorProto_TYPE_ENUM {
		// We avoid using obj.JavaPackageName(), because we want to use the
		// original (proto-world) package name.
		obj := g.ObjectNamed(field.GetTypeName())
		if id, ok := obj.(*ImportedDescriptor); ok {
			obj = id.o
		}
		enum = ",enum="
		if pkg := obj.File().GetPackage(); pkg != "" {
			enum += pkg + "."
		}
		enum += CamelCaseSlice(obj.TypeName())
	}
	packed := ""
	if (field.Options != nil && field.Options.GetPacked()) ||
		// Per https://developers.google.com/protocol-buffers/docs/proto3#simple:
		// "In proto3, repeated fields of scalar numeric types use packed encoding by default."
		(message.proto3() && (field.Options == nil || field.Options.Packed == nil) &&
			isRepeated(field) && isScalar(field)) {
		packed = ",packed"
	}
	fieldName := field.GetName()
	name := fieldName
	if *field.Type == descriptor.FieldDescriptorProto_TYPE_GROUP {
		// We must use the type name for groups instead of
		// the field name to preserve capitalization.
		// type_name in FieldDescriptorProto is fully-qualified,
		// but we only want the local part.
		name = *field.TypeName
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
	}
	if json := field.GetJsonName(); field.Extendee == nil && json != "" && json != name {
		// TODO: escaping might be needed, in which case
		// perhaps this should be in its own "json" tag.
		name += ",json=" + json
	}
	name = ",name=" + name
	if message.proto3() {
		name += ",proto3"
	}
	oneof := ""
	if field.OneofIndex != nil {
		oneof = ",oneof"
	}
	return strconv.Quote(fmt.Sprintf("%s,%d,%s%s%s%s%s%s",
		wiretype,
		field.GetNumber(),
		optrepreq,
		packed,
		name,
		enum,
		oneof,
		defaultValue))
}

// TypeName is the printed name appropriate for an item. If the object is in the current file,
// TypeName drops the package name and underscores the rest.
// Otherwise the object is from another package; and the result is the underscored
// package name followed by the item name.
// The result always has an initial capital.
func (g *Generator) TypeName(obj Object) string {
	return g.DefaultPackageName(obj) + CamelCaseSlice(obj.TypeName())
}

// GoType returns a string representing the type name, and the wire type
func (g *Generator) GoType(message *Descriptor, field *descriptor.FieldDescriptorProto) (typ string, wire string) {
	// TODO: Options.
	switch *field.Type {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		typ, wire = "float64", "fixed64"
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		typ, wire = "float32", "fixed32"
	case descriptor.FieldDescriptorProto_TYPE_INT64:
		typ, wire = "int64", "varint"
	case descriptor.FieldDescriptorProto_TYPE_UINT64:
		typ, wire = "uint64", "varint"
	case descriptor.FieldDescriptorProto_TYPE_INT32:
		typ, wire = "int32", "varint"
	case descriptor.FieldDescriptorProto_TYPE_UINT32:
		typ, wire = "uint32", "varint"
	case descriptor.FieldDescriptorProto_TYPE_FIXED64:
		typ, wire = "uint64", "fixed64"
	case descriptor.FieldDescriptorProto_TYPE_FIXED32:
		typ, wire = "uint32", "fixed32"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		typ, wire = "bool", "varint"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		typ, wire = "string", "bytes"
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		desc := g.ObjectNamed(field.GetTypeName())
		typ, wire = "*"+g.TypeName(desc), "group"
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		desc := g.ObjectNamed(field.GetTypeName())
		typ, wire = "*"+g.TypeName(desc), "bytes"
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		typ, wire = "[]byte", "bytes"
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		desc := g.ObjectNamed(field.GetTypeName())
		typ, wire = g.TypeName(desc), "varint"
	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		typ, wire = "int32", "fixed32"
	case descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		typ, wire = "int64", "fixed64"
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		typ, wire = "int32", "zigzag32"
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		typ, wire = "int64", "zigzag64"
	default:
		g.Fail("unknown type for", field.GetName())
	}
	if isRepeated(field) {
		typ = "[]" + typ
	} else if message != nil && message.proto3() {
		return
	} else if field.OneofIndex != nil && message != nil {
		return
	} else if needsStar(*field.Type) {
		typ = "*" + typ
	}
	return
}

func (g *Generator) RecordTypeUse(t string) {
	if _, ok := g.typeNameToObject[t]; !ok {
		return
	}
	importPath := g.ObjectNamed(t).JavaImportPath()
	if importPath == g.outputImportPath {
		// Don't record use of objects in our package.
		return
	}
	g.AddImport(importPath)
	g.usedPackages[importPath] = true
}

// Method names that may be generated.  Fields with these names get an
// underscore appended. Any change to this set is a potential incompatible
// API change because it changes generated field names.
var methodNames = [...]string{
	"Reset",
	"String",
	"ProtoMessage",
	"Marshal",
	"Unmarshal",
	"ExtensionRangeArray",
	"ExtensionMap",
	"Descriptor",
}

// Names of messages in the `google.protobuf` package for which
// we will generate XXX_WellKnownType methods.
var wellKnownTypes = map[string]bool{
	"Any":       true,
	"Duration":  true,
	"Empty":     true,
	"Struct":    true,
	"Timestamp": true,

	"Value":       true,
	"ListValue":   true,
	"DoubleValue": true,
	"FloatValue":  true,
	"Int64Value":  true,
	"UInt64Value": true,
	"Int32Value":  true,
	"UInt32Value": true,
	"BoolValue":   true,
	"StringValue": true,
	"BytesValue":  true,
}

// getterDefault finds the default value for the field to return from a getter,
// regardless of if it's a built in default or explicit from the source. Returns e.g. "nil", `""`, "Default_MessageType_FieldName"
func (g *Generator) getterDefault(field *descriptor.FieldDescriptorProto, goMessageType string) string {
	if isRepeated(field) {
		return "nil"
	}
	if def := field.GetDefaultValue(); def != "" {
		defaultConstant := g.defaultConstantName(goMessageType, field.GetName())
		if *field.Type != descriptor.FieldDescriptorProto_TYPE_BYTES {
			return defaultConstant
		}
		return "append([]byte(nil), " + defaultConstant + "...)"
	}
	switch *field.Type {
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		return "false"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		return `""`
	case descriptor.FieldDescriptorProto_TYPE_GROUP, descriptor.FieldDescriptorProto_TYPE_MESSAGE, descriptor.FieldDescriptorProto_TYPE_BYTES:
		return "nil"
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		obj := g.ObjectNamed(field.GetTypeName())
		var enum *EnumDescriptor
		if id, ok := obj.(*ImportedDescriptor); ok {
			// The enum type has been publicly imported.
			enum, _ = id.o.(*EnumDescriptor)
		} else {
			enum, _ = obj.(*EnumDescriptor)
		}
		if enum == nil {
			log.Printf("don't know how to generate getter for %s", field.GetName())
			return "nil"
		}
		if len(enum.Value) == 0 {
			return "0 // empty enum"
		}
		first := enum.Value[0].GetName()
		return g.DefaultPackageName(obj) + enum.prefix() + first
	default:
		return "0"
	}
}

// defaultConstantName builds the name of the default constant from the message
// type name and the untouched field name, e.g. "Default_MessageType_FieldName"
func (g *Generator) defaultConstantName(goMessageType, protoFieldName string) string {
	return "Default_" + goMessageType + "_" + CamelCase(protoFieldName)
}

// The different types of fields in a message and how to actually print them
// Most of the logic for generateMessage is in the methods of these types.
//
// Note that the content of the field is irrelevant, a simpleField can contain
// anything from a scalar to a group (which is just a message).
//
// Extension fields (and message sets) are however handled separately.
//
// simpleField - a field that is neiter weak nor oneof, possibly repeated
// oneofField - field containing list of subfields:
// - oneofSubField - a field within the oneof

// msgCtx contains the context for the generator functions.
type msgCtx struct {
	goName  string      // Go struct name of the message, e.g. MessageName
	message *Descriptor // The descriptor for the message
}

// fieldCommon contains data common to all types of fields.
type fieldCommon struct {
	goName     string // Go name of field, e.g. "FieldName" or "Descriptor_"
	protoName  string // Name of field in proto language, e.g. "field_name" or "descriptor"
	getterName string // Name of the getter, e.g. "GetFieldName" or "GetDescriptor_"
	goType     string // The Go type as a string, e.g. "*int32" or "*OtherMessage"
	tags       string // The tag string/annotation for the type, e.g. `protobuf:"varint,8,opt,name=region_id,json=regionId"`
	fullPath   string // The full path of the field as used by Annotate etc, e.g. "4,0,2,0"
}

// getProtoName gets the proto name of a field, e.g. "field_name" or "descriptor".
func (f *fieldCommon) getProtoName() string {
	return f.protoName
}

// getGoType returns the go type of the field  as a string, e.g. "*int32".
func (f *fieldCommon) getGoType() string {
	return f.goType
}

// simpleField is not weak, not a oneof, not an extension. Can be required, optional or repeated.
type simpleField struct {
	fieldCommon
	protoTypeName string                               // Proto type name, empty if primitive, e.g. ".google.protobuf.Duration"
	protoType     descriptor.FieldDescriptorProto_Type // Actual type enum value, e.g. descriptor.FieldDescriptorProto_TYPE_FIXED64
	deprecated    string                               // Deprecation comment, if any, e.g. "// Deprecated: Do not use."
	getterDef     string                               // Default for getters, e.g. "nil", `""` or "Default_MessageType_FieldName"
	protoDef      string                               // Default value as defined in the proto file, e.g "yoshi" or "5"
	comment       string                               // The full comment for the field, e.g. "// Useful information"
}

// decl prints the declaration of the field in the struct (if any).
func (f *simpleField) decl(g *Generator, mc *msgCtx) {
	g.P(f.comment, Annotate(mc.message.file, f.fullPath, f.goName), "\t", f.goType, "\t`", f.tags, "`", f.deprecated)
}

// getter prints the getter for the field.
func (f *simpleField) getter(g *Generator, mc *msgCtx) {
	star := ""
	tname := f.goType
	if needsStar(f.protoType) && tname[0] == '*' {
		tname = tname[1:]
		star = "*"
	}
	if f.deprecated != "" {
		g.P(f.deprecated)
	}
	g.P("func (m *", mc.goName, ") ", Annotate(mc.message.file, f.fullPath, f.getterName), "() "+tname+" {")
	if f.getterDef == "nil" { // Simpler getter
		g.P("if m != nil {")
		g.P("return m." + f.goName)
		g.P("}")
		g.P("return nil")
		g.P("}")
		g.P()
		return
	}
	if mc.message.proto3() {
		g.P("if m != nil {")
	} else {
		g.P("if m != nil && m." + f.goName + " != nil {")
	}
	g.P("return " + star + "m." + f.goName)
	g.P("}")
	g.P("return ", f.getterDef)
	g.P("}")
	g.P()
}

// setter prints the setter method of the field.
func (f *simpleField) setter(g *Generator, mc *msgCtx) {
	// No setter for regular fields yet
}

// getProtoDef returns the default value explicitly stated in the proto file, e.g "yoshi" or "5".
func (f *simpleField) getProtoDef() string {
	return f.protoDef
}

// getProtoTypeName returns the protobuf type name for the field as returned by field.GetTypeName(), e.g. ".google.protobuf.Duration".
func (f *simpleField) getProtoTypeName() string {
	return f.protoTypeName
}

// getProtoType returns the *field.Type value, e.g. descriptor.FieldDescriptorProto_TYPE_FIXED64.
func (f *simpleField) getProtoType() descriptor.FieldDescriptorProto_Type {
	return f.protoType
}

// oneofSubFields are kept slize held by each oneofField. They do not appear in the top level slize of fields for the message.
type oneofSubField struct {
	fieldCommon
	protoTypeName string                               // Proto type name, empty if primitive, e.g. ".google.protobuf.Duration"
	protoType     descriptor.FieldDescriptorProto_Type // Actual type enum value, e.g. descriptor.FieldDescriptorProto_TYPE_FIXED64
	oneofTypeName string                               // Type name of the enclosing struct, e.g. "MessageName_FieldName"
	fieldNumber   int                                  // Actual field number, as defined in proto, e.g. 12
	getterDef     string                               // Default for getters, e.g. "nil", `""` or "Default_MessageType_FieldName"
	protoDef      string                               // Default value as defined in the proto file, e.g "yoshi" or "5"
	deprecated    string                               // Deprecation comment, if any.
}

// wireTypeName returns a textual wire type, needed for oneof sub fields in generated code.
func (f *oneofSubField) wireTypeName() string {
	switch f.protoType {
	case descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		return "WireFixed64"
	case descriptor.FieldDescriptorProto_TYPE_FIXED32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_FLOAT:
		return "WireFixed32"
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		return "WireStartGroup"
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE,
		descriptor.FieldDescriptorProto_TYPE_STRING,
		descriptor.FieldDescriptorProto_TYPE_BYTES:
		return "WireBytes"
	default: // All others are Varints
		return "WireVarint"
	}
}

// typedNil prints a nil casted to the pointer to this field.
// - for XXX_OneofFuncs
func (f *oneofSubField) typedNil(g *Generator) {
	g.P("(*", f.oneofTypeName, ")(nil),")
}

// marshalCase prints the case matching this oneof subfield in the marshalling code.
func (f *oneofSubField) marshalCase(g *Generator) {
	g.P("case *", f.oneofTypeName, ":")
	wire := f.wireTypeName()
	var pre, post string
	val := "x." + f.goName // overridden for TYPE_BOOL
	switch f.protoType {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		pre = "b.EncodeFixed64(" + g.Pkg["math"] + ".Float64bits("
		post = "))"
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		pre = "b.EncodeFixed32(uint64(" + g.Pkg["math"] + ".Float32bits("
		post = ")))"
	case descriptor.FieldDescriptorProto_TYPE_INT64, descriptor.FieldDescriptorProto_TYPE_UINT64:
		pre, post = "b.EncodeVarint(uint64(", "))"
	case descriptor.FieldDescriptorProto_TYPE_INT32, descriptor.FieldDescriptorProto_TYPE_UINT32, descriptor.FieldDescriptorProto_TYPE_ENUM:
		pre, post = "b.EncodeVarint(uint64(", "))"
	case descriptor.FieldDescriptorProto_TYPE_FIXED64, descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		pre, post = "b.EncodeFixed64(uint64(", "))"
	case descriptor.FieldDescriptorProto_TYPE_FIXED32, descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		pre, post = "b.EncodeFixed32(uint64(", "))"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		g.P("t := uint64(0)")
		g.P("if ", val, " { t = 1 }")
		val = "t"
		pre, post = "b.EncodeVarint(", ")"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		pre, post = "b.EncodeStringBytes(", ")"
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		pre, post = "b.Marshal(", ")"
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		pre, post = "b.EncodeMessage(", ")"
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		pre, post = "b.EncodeRawBytes(", ")"
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		pre, post = "b.EncodeZigzag32(uint64(", "))"
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		pre, post = "b.EncodeZigzag64(uint64(", "))"
	default:
		g.Fail("unhandled oneof field type ", f.protoType.String())
	}
	g.P("b.EncodeVarint(", f.fieldNumber, "<<3|", g.Pkg["proto"], ".", wire, ")")
	if t := f.protoType; t != descriptor.FieldDescriptorProto_TYPE_GROUP && t != descriptor.FieldDescriptorProto_TYPE_MESSAGE {
		g.P(pre, val, post)
	} else {
		g.P("if err := ", pre, val, post, "; err != nil {")
		g.P("return err")
		g.P("}")
	}
	if f.protoType == descriptor.FieldDescriptorProto_TYPE_GROUP {
		g.P("b.EncodeVarint(", f.fieldNumber, "<<3|", g.Pkg["proto"], ".WireEndGroup)")
	}
}

// unmarshalCase prints the case matching this oneof subfield in the unmarshalling code.
func (f *oneofSubField) unmarshalCase(g *Generator, origOneofName string, oneofName string) {
	g.P("case ", f.fieldNumber, ": // ", origOneofName, ".", f.getProtoName())
	g.P("if wire != ", g.Pkg["proto"], ".", f.wireTypeName(), " {")
	g.P("return true, ", g.Pkg["proto"], ".ErrInternalBadWireType")
	g.P("}")
	lhs := "x, err" // overridden for TYPE_MESSAGE and TYPE_GROUP
	var dec, cast, cast2 string
	switch f.protoType {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		dec, cast = "b.DecodeFixed64()", g.Pkg["math"]+".Float64frombits"
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		dec, cast, cast2 = "b.DecodeFixed32()", "uint32", g.Pkg["math"]+".Float32frombits"
	case descriptor.FieldDescriptorProto_TYPE_INT64:
		dec, cast = "b.DecodeVarint()", "int64"
	case descriptor.FieldDescriptorProto_TYPE_UINT64:
		dec = "b.DecodeVarint()"
	case descriptor.FieldDescriptorProto_TYPE_INT32:
		dec, cast = "b.DecodeVarint()", "int32"
	case descriptor.FieldDescriptorProto_TYPE_FIXED64:
		dec = "b.DecodeFixed64()"
	case descriptor.FieldDescriptorProto_TYPE_FIXED32:
		dec, cast = "b.DecodeFixed32()", "uint32"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		dec = "b.DecodeVarint()"
		// handled specially below
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		dec = "b.DecodeStringBytes()"
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		g.P("msg := new(", f.goType[1:], ")") // drop star
		lhs = "err"
		dec = "b.DecodeGroup(msg)"
		// handled specially below
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		g.P("msg := new(", f.goType[1:], ")") // drop star
		lhs = "err"
		dec = "b.DecodeMessage(msg)"
		// handled specially below
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		dec = "b.DecodeRawBytes(true)"
	case descriptor.FieldDescriptorProto_TYPE_UINT32:
		dec, cast = "b.DecodeVarint()", "uint32"
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		dec, cast = "b.DecodeVarint()", f.goType
	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		dec, cast = "b.DecodeFixed32()", "int32"
	case descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		dec, cast = "b.DecodeFixed64()", "int64"
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		dec, cast = "b.DecodeZigzag32()", "int32"
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		dec, cast = "b.DecodeZigzag64()", "int64"
	default:
		g.Fail("unhandled oneof field type ", f.protoType.String())
	}
	g.P(lhs, " := ", dec)
	val := "x"
	if cast != "" {
		val = cast + "(" + val + ")"
	}
	if cast2 != "" {
		val = cast2 + "(" + val + ")"
	}
	switch f.protoType {
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		val += " != 0"
	case descriptor.FieldDescriptorProto_TYPE_GROUP,
		descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		val = "msg"
	}
	g.P("m.", oneofName, " = &", f.oneofTypeName, "{", val, "}")
	g.P("return true, err")
}

// sizerCase prints the case matching this oneof subfield in the sizer code.
func (f *oneofSubField) sizerCase(g *Generator) {
	g.P("case *", f.oneofTypeName, ":")
	val := "x." + f.goName
	var varint, fixed string
	switch f.protoType {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		fixed = "8"
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		fixed = "4"
	case descriptor.FieldDescriptorProto_TYPE_INT64, descriptor.FieldDescriptorProto_TYPE_UINT64, descriptor.FieldDescriptorProto_TYPE_INT32, descriptor.FieldDescriptorProto_TYPE_UINT32, descriptor.FieldDescriptorProto_TYPE_ENUM:
		varint = val
	case descriptor.FieldDescriptorProto_TYPE_FIXED64, descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		fixed = "8"
	case descriptor.FieldDescriptorProto_TYPE_FIXED32, descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		fixed = "4"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		fixed = "1"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		fixed = "len(" + val + ")"
		varint = fixed
	case descriptor.FieldDescriptorProto_TYPE_GROUP:
		fixed = g.Pkg["proto"] + ".Size(" + val + ")"
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		g.P("s := ", g.Pkg["proto"], ".Size(", val, ")")
		fixed = "s"
		varint = fixed
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		fixed = "len(" + val + ")"
		varint = fixed
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		varint = "(uint32(" + val + ") << 1) ^ uint32((int32(" + val + ") >> 31))"
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		varint = "uint64(" + val + " << 1) ^ uint64((int64(" + val + ") >> 63))"
	default:
		g.Fail("unhandled oneof field type ", f.protoType.String())
	}
	// Tag and wire varint is known statically,
	// so don't generate code for that part of the size computation.
	tagAndWireSize := proto.SizeVarint(uint64(f.fieldNumber << 3)) // wire doesn't affect varint size
	g.P("n += ", tagAndWireSize, " // tag and wire")
	if varint != "" {
		g.P("n += ", g.Pkg["proto"], ".SizeVarint(uint64(", varint, "))")
	}
	if fixed != "" {
		g.P("n += ", fixed)
	}
	if f.protoType == descriptor.FieldDescriptorProto_TYPE_GROUP {
		g.P("n += ", tagAndWireSize, " // tag and wire")
	}
}

// getProtoDef returns the default value explicitly stated in the proto file, e.g "yoshi" or "5".
func (f *oneofSubField) getProtoDef() string {
	return f.protoDef
}

// getProtoTypeName returns the protobuf type name for the field as returned by field.GetTypeName(), e.g. ".google.protobuf.Duration".
func (f *oneofSubField) getProtoTypeName() string {
	return f.protoTypeName
}

// getProtoType returns the *field.Type value, e.g. descriptor.FieldDescriptorProto_TYPE_FIXED64.
func (f *oneofSubField) getProtoType() descriptor.FieldDescriptorProto_Type {
	return f.protoType
}

// oneofField represents the oneof on top level.
// The alternative fields within the oneof are represented by oneofSubField.
type oneofField struct {
	fieldCommon
	subFields []*oneofSubField // All the possible oneof fields
	comment   string           // The full comment for the field, e.g. "// Types that are valid to be assigned to MyOneof:\n\\"
}

// decl prints the declaration of the field in the struct (if any).
func (f *oneofField) decl(g *Generator, mc *msgCtx) {
	comment := f.comment
	for _, sf := range f.subFields {
		comment += "//\t*" + sf.oneofTypeName + "\n"
	}
	g.P(comment, Annotate(mc.message.file, f.fullPath, f.goName), " ", f.goType, " `", f.tags, "`")
}

// getter for a oneof field will print additional discriminators and interfaces for the oneof,
// also it prints all the getters for the sub fields.
func (f *oneofField) getter(g *Generator, mc *msgCtx) {
	// The discriminator type
	g.P("type ", f.goType, " interface {")
	g.P(f.goType, "()")
	g.P("}")
	g.P()
	// The subField types, fulfilling the discriminator type contract
	for _, sf := range f.subFields {
		g.P("type ", Annotate(mc.message.file, sf.fullPath, sf.oneofTypeName), " struct {")
		g.P(Annotate(mc.message.file, sf.fullPath, sf.goName), " ", sf.goType, " `", sf.tags, "`")
		g.P("}")
		g.P()
	}
	for _, sf := range f.subFields {
		g.P("func (*", sf.oneofTypeName, ") ", f.goType, "() {}")
		g.P()
	}
	// Getter for the oneof field
	g.P("func (m *", mc.goName, ") ", Annotate(mc.message.file, f.fullPath, f.getterName), "() ", f.goType, " {")
	g.P("if m != nil { return m.", f.goName, " }")
	g.P("return nil")
	g.P("}")
	g.P()
	// Getters for each oneof
	for _, sf := range f.subFields {
		if sf.deprecated != "" {
			g.P(sf.deprecated)
		}
		g.P("func (m *", mc.goName, ") ", Annotate(mc.message.file, sf.fullPath, sf.getterName), "() "+sf.goType+" {")
		g.P("if x, ok := m.", f.getterName, "().(*", sf.oneofTypeName, "); ok {")
		g.P("return x.", sf.goName)
		g.P("}")
		g.P("return ", sf.getterDef)
		g.P("}")
		g.P()
	}
}

// setter prints the setter method of the field.
func (f *oneofField) setter(g *Generator, mc *msgCtx) {
	// No setters for oneof yet
}

// topLevelField interface implemented by all types of fields on the top level (not oneofSubField).
type topLevelField interface {
	decl(g *Generator, mc *msgCtx)   // print declaration within the struct
	getter(g *Generator, mc *msgCtx) // print getter
	setter(g *Generator, mc *msgCtx) // print setter if applicable
}

// defField interface implemented by all types of fields that can have defaults (not oneofField, but instead oneofSubField).
type defField interface {
	getProtoDef() string                                // default value explicitly stated in the proto file, e.g "yoshi" or "5"
	getProtoName() string                               // proto name of a field, e.g. "field_name" or "descriptor"
	getGoType() string                                  // go type of the field  as a string, e.g. "*int32"
	getProtoTypeName() string                           // protobuf type name for the field, e.g. ".google.protobuf.Duration"
	getProtoType() descriptor.FieldDescriptorProto_Type // *field.Type value, e.g. descriptor.FieldDescriptorProto_TYPE_FIXED64
}

// generateDefaultConstants adds constants for default values if needed, which is only if the default value is.
// explicit in the proto.
func (g *Generator) generateDefaultConstants(mc *msgCtx, topLevelFields []topLevelField) {
	// Collect fields that can have defaults
	dFields := []defField{}
	for _, pf := range topLevelFields {
		if f, ok := pf.(*oneofField); ok {
			for _, osf := range f.subFields {
				dFields = append(dFields, osf)
			}
			continue
		}
		dFields = append(dFields, pf.(defField))
	}
	for _, df := range dFields {
		def := df.getProtoDef()
		if def == "" {
			continue
		}
		fieldname := g.defaultConstantName(mc.goName, df.getProtoName())
		typename := df.getGoType()
		if typename[0] == '*' {
			typename = typename[1:]
		}
		kind := "const "
		switch {
		case typename == "bool":
		case typename == "string":
			def = strconv.Quote(def)
		case typename == "[]byte":
			def = "[]byte(" + strconv.Quote(unescape(def)) + ")"
			kind = "var "
		case def == "inf", def == "-inf", def == "nan":
			// These names are known to, and defined by, the protocol language.
			switch def {
			case "inf":
				def = "math.Inf(1)"
			case "-inf":
				def = "math.Inf(-1)"
			case "nan":
				def = "math.NaN()"
			}
			if df.getProtoType() == descriptor.FieldDescriptorProto_TYPE_FLOAT {
				def = "float32(" + def + ")"
			}
			kind = "var "
		case df.getProtoType() == descriptor.FieldDescriptorProto_TYPE_FLOAT:
			if f, err := strconv.ParseFloat(def, 32); err == nil {
				def = fmt.Sprint(float32(f))
			}
		case df.getProtoType() == descriptor.FieldDescriptorProto_TYPE_DOUBLE:
			if f, err := strconv.ParseFloat(def, 64); err == nil {
				def = fmt.Sprint(f)
			}
		case df.getProtoType() == descriptor.FieldDescriptorProto_TYPE_ENUM:
			// Must be an enum.  Need to construct the prefixed name.
			obj := g.ObjectNamed(df.getProtoTypeName())
			var enum *EnumDescriptor
			if id, ok := obj.(*ImportedDescriptor); ok {
				// The enum type has been publicly imported.
				enum, _ = id.o.(*EnumDescriptor)
			} else {
				enum, _ = obj.(*EnumDescriptor)
			}
			if enum == nil {
				log.Printf("don't know how to generate constant for %s", fieldname)
				continue
			}
			def = g.DefaultPackageName(obj) + enum.prefix() + def
		}
		g.P(kind, fieldname, " ", typename, " = ", def)
		g.file.addExport(mc.message, constOrVarSymbol{fieldname, kind, ""})
	}
	g.P()
}

// generateInternalStructFields just adds the XXX_<something> fields to the message struct.
func (g *Generator) generateInternalStructFields(mc *msgCtx, topLevelFields []topLevelField) {
	g.P("XXX_NoUnkeyedLiteral\tstruct{} `json:\"-\"`") // prevent unkeyed struct literals
	if len(mc.message.ExtensionRange) > 0 {
		messageset := ""
		if opts := mc.message.Options; opts != nil && opts.GetMessageSetWireFormat() {
			messageset = "protobuf_messageset:\"1\" "
		}
		g.P(g.Pkg["proto"], ".XXX_InternalExtensions `", messageset, "json:\"-\"`")
	}
	g.P("XXX_unrecognized\t[]byte `json:\"-\"`")
	g.P("XXX_sizecache\tint32 `json:\"-\"`")

}

// generateOneofFuncs adds all the utility functions for oneof, including marshalling, unmarshalling and sizer.
func (g *Generator) generateOneofFuncs(mc *msgCtx, topLevelFields []topLevelField) {
	ofields := []*oneofField{}
	for _, f := range topLevelFields {
		if o, ok := f.(*oneofField); ok {
			ofields = append(ofields, o)
		}
	}
	if len(ofields) == 0 {
		return
	}
	enc := "_" + mc.goName + "_OneofMarshaler"
	dec := "_" + mc.goName + "_OneofUnmarshaler"
	size := "_" + mc.goName + "_OneofSizer"
	encSig := "(msg " + g.Pkg["proto"] + ".Message, b *" + g.Pkg["proto"] + ".Buffer) error"
	decSig := "(msg " + g.Pkg["proto"] + ".Message, tag, wire int, b *" + g.Pkg["proto"] + ".Buffer) (bool, error)"
	sizeSig := "(msg " + g.Pkg["proto"] + ".Message) (n int)"

	// OneofFuncs
	g.P("// XXX_OneofFuncs is for the internal use of the proto package.")
	g.P("func (*", mc.goName, ") XXX_OneofFuncs() (func", encSig, ", func", decSig, ", func", sizeSig, ", []interface{}) {")
	g.P("return ", enc, ", ", dec, ", ", size, ", []interface{}{")
	for _, of := range ofields {
		for _, sf := range of.subFields {
			sf.typedNil(g)
		}
	}
	g.P("}")
	g.P("}")
	g.P()

	// marshaler
	g.P("func ", enc, encSig, " {")
	g.P("m := msg.(*", mc.goName, ")")
	for _, of := range ofields {
		g.P("// ", of.getProtoName())
		g.P("switch x := m.", of.goName, ".(type) {")
		for _, sf := range of.subFields {
			// also fills in field.wire
			sf.marshalCase(g)
		}
		g.P("case nil:")
		g.P("default:")
		g.P(" return ", g.Pkg["fmt"], `.Errorf("`, mc.goName, ".", of.goName, ` has unexpected type %T", x)`)
		g.P("}")
	}
	g.P("return nil")
	g.P("}")
	g.P()

	// unmarshaler
	g.P("func ", dec, decSig, " {")
	g.P("m := msg.(*", mc.goName, ")")
	g.P("switch tag {")
	for _, of := range ofields {
		for _, sf := range of.subFields {
			sf.unmarshalCase(g, of.getProtoName(), of.goName)
		}
	}
	g.P("default:")
	g.P("return false, nil")
	g.P("}")
	g.P("}")
	g.P()

	// sizer
	g.P("func ", size, sizeSig, " {")
	g.P("m := msg.(*", mc.goName, ")")
	for _, of := range ofields {
		g.P("// ", of.getProtoName())
		g.P("switch x := m.", of.goName, ".(type) {")
		for _, sf := range of.subFields {
			// also fills in field.wire
			sf.sizerCase(g)
		}
		g.P("case nil:")
		g.P("default:")
		g.P("panic(", g.Pkg["fmt"], ".Sprintf(\"proto: unexpected type %T in oneof\", x))")
		g.P("}")
	}
	g.P("return n")
	g.P("}")
	g.P()
}

// generateMessageStruct adds the actual struct with it's members (but not methods) to the output.
func (g *Generator) generateMessageStruct(mc *msgCtx, topLevelFields []topLevelField) {
	comments := g.PrintComments(mc.message.path)

	// Guarantee deprecation comments appear after user-provided comments.
	if mc.message.GetOptions().GetDeprecated() {
		if comments {
			// Convention: Separate deprecation comments from original
			// comments with an empty line.
			g.P("//")
		}
		g.P(deprecationComment)
	}

	g.P("type ", Annotate(mc.message.file, mc.message.path, mc.goName), " struct {")
	for _, pf := range topLevelFields {
		pf.decl(g, mc)
	}
	g.generateInternalStructFields(mc, topLevelFields)
	g.P("}")
}

// generateGetters adds getters for all fields, including oneofs and weak fields when applicable.
func (g *Generator) generateGetters(mc *msgCtx, topLevelFields []topLevelField) {
	for _, pf := range topLevelFields {
		pf.getter(g, mc)
	}
}

// generateSetters add setters for all fields, including oneofs and weak fields when applicable.
func (g *Generator) generateSetters(mc *msgCtx, topLevelFields []topLevelField) {
	for _, pf := range topLevelFields {
		pf.setter(g, mc)
	}
}

// generateCommonMethods adds methods to the message that are not on a per field basis.
func (g *Generator) generateCommonMethods(mc *msgCtx) {
	// Reset, String and ProtoMessage methods.
	g.P("func (m *", mc.goName, ") Reset() { *m = ", mc.goName, "{} }")
	g.P("func (m *", mc.goName, ") String() string { return ", g.Pkg["proto"], ".CompactTextString(m) }")
	g.P("func (*", mc.goName, ") ProtoMessage() {}")
	var indexes []string
	for m := mc.message; m != nil; m = m.parent {
		indexes = append([]string{strconv.Itoa(m.index)}, indexes...)
	}
	g.P("func (*", mc.goName, ") Descriptor() ([]byte, []int) {")
	g.P("return ", g.file.VarName(), ", []int{", strings.Join(indexes, ", "), "}")
	g.P("}")
	g.P()
	// TODO: Revisit the decision to use a XXX_WellKnownType method
	// if we change proto.MessageName to work with multiple equivalents.
	if mc.message.file.GetPackage() == "google.protobuf" && wellKnownTypes[mc.message.GetName()] {
		g.P("func (*", mc.goName, `) XXX_WellKnownType() string { return "`, mc.message.GetName(), `" }`)
		g.P()
	}

	// Extension support methods
	if len(mc.message.ExtensionRange) > 0 {
		// message_set_wire_format only makes sense when extensions are defined.
		if opts := mc.message.Options; opts != nil && opts.GetMessageSetWireFormat() {
			g.P()
			g.P("func (m *", mc.goName, ") MarshalJSON() ([]byte, error) {")
			g.P("return ", g.Pkg["proto"], ".MarshalMessageSetJSON(&m.XXX_InternalExtensions)")
			g.P("}")
			g.P("func (m *", mc.goName, ") UnmarshalJSON(buf []byte) error {")
			g.P("return ", g.Pkg["proto"], ".UnmarshalMessageSetJSON(buf, &m.XXX_InternalExtensions)")
			g.P("}")
		}

		g.P()
		g.P("var extRange_", mc.goName, " = []", g.Pkg["proto"], ".ExtensionRange{")
		for _, r := range mc.message.ExtensionRange {
			end := fmt.Sprint(*r.End - 1) // make range inclusive on both ends
			g.P("{Start: ", r.Start, ", End: ", end, "},")
		}
		g.P("}")
		g.P("func (*", mc.goName, ") ExtensionRangeArray() []", g.Pkg["proto"], ".ExtensionRange {")
		g.P("return extRange_", mc.goName)
		g.P("}")
		g.P()
	}

	// TODO: It does not scale to keep adding another method for every
	// operation on protos that we want to switch over to using the
	// table-driven approach. Instead, we should only add a single method
	// that allows getting access to the *InternalMessageInfo struct and then
	// calling Unmarshal, Marshal, Merge, Size, and Discard directly on that.

	// Wrapper for table-driven marshaling and unmarshaling.
	g.P("func (m *", mc.goName, ") XXX_Unmarshal(b []byte) error {")
	g.P("return xxx_messageInfo_", mc.goName, ".Unmarshal(m, b)")
	g.P("}")

	g.P("func (m *", mc.goName, ") XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {")
	g.P("return xxx_messageInfo_", mc.goName, ".Marshal(b, m, deterministic)")
	g.P("}")

	g.P("func (m *", mc.goName, ") XXX_Merge(src ", g.Pkg["proto"], ".Message) {")
	g.P("xxx_messageInfo_", mc.goName, ".Merge(m, src)")
	g.P("}")

	g.P("func (m *", mc.goName, ") XXX_Size() int {") // avoid name clash with "Size" field in some message
	g.P("return xxx_messageInfo_", mc.goName, ".Size(m)")
	g.P("}")

	g.P("func (m *", mc.goName, ") XXX_DiscardUnknown() {")
	g.P("xxx_messageInfo_", mc.goName, ".DiscardUnknown(m)")
	g.P("}")

	g.P("var xxx_messageInfo_", mc.goName, " ", g.Pkg["proto"], ".InternalMessageInfo")
	g.P()
}

// Generate the type, methods and default constant definitions for this Descriptor.
func (g *Generator) generateMessage(message *Descriptor) {
	topLevelFields := []topLevelField{}
	oFields := make(map[int32]*oneofField)
	// The full type name
	typeName := message.TypeName()
	// The full type name, CamelCased.
	goTypeName := CamelCaseSlice(typeName)

	usedNames := make(map[string]bool)
	for _, n := range methodNames {
		usedNames[n] = true
	}

	// allocNames finds a conflict-free variation of the given strings,
	// consistently mutating their suffixes.
	// It returns the same number of strings.
	allocNames := func(ns ...string) []string {
	Loop:
		for {
			for _, n := range ns {
				if usedNames[n] {
					for i := range ns {
						ns[i] += "_"
					}
					continue Loop
				}
			}
			for _, n := range ns {
				usedNames[n] = true
			}
			return ns
		}
	}

	mapFieldTypes := make(map[*descriptor.FieldDescriptorProto]string) // keep track of the map fields to be added later

	// Build a structure more suitable for generating the text in one pass
	for i, field := range message.Field {
		// Allocate the getter and the field at the same time so name
		// collisions create field/method consistent names.
		// TODO: This allocation occurs based on the order of the fields
		// in the proto file, meaning that a change in the field
		// ordering can change generated Method/Field names.
		base := CamelCase(*field.Name)
		ns := allocNames(base, "Get"+base)
		fieldName, fieldGetterName := ns[0], ns[1]
		typename, wiretype := g.GoType(message, field)
		jsonName := *field.Name
		tag := fmt.Sprintf("protobuf:%s json:%q", g.goTag(message, field, wiretype), jsonName+",omitempty")

		oneof := field.OneofIndex != nil
		if oneof && oFields[*field.OneofIndex] == nil {
			odp := message.OneofDecl[int(*field.OneofIndex)]
			base := CamelCase(odp.GetName())
			names := allocNames(base, "Get"+base)
			fname, gname := names[0], names[1]

			// This is the first field of a oneof we haven't seen before.
			// Generate the union field.
			oneofFullPath := fmt.Sprintf("%s,%d,%d", message.path, messageOneofPath, *field.OneofIndex)
			c, ok := g.makeComments(oneofFullPath)
			if ok {
				c += "\n//\n"
			}
			c += "// Types that are valid to be assigned to " + fname + ":\n"
			// Generate the rest of this comment later,
			// when we've computed any disambiguation.

			dname := "is" + goTypeName + "_" + fname
			tag := `protobuf_oneof:"` + odp.GetName() + `"`
			of := oneofField{
				fieldCommon: fieldCommon{
					goName:     fname,
					getterName: gname,
					goType:     dname,
					tags:       tag,
					protoName:  odp.GetName(),
					fullPath:   oneofFullPath,
				},
				comment: c,
			}
			topLevelFields = append(topLevelFields, &of)
			oFields[*field.OneofIndex] = &of
		}

		if *field.Type == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
			desc := g.ObjectNamed(field.GetTypeName())
			if d, ok := desc.(*Descriptor); ok && d.GetOptions().GetMapEntry() {
				// Figure out the Go types and tags for the key and value types.
				keyField, valField := d.Field[0], d.Field[1]
				keyType, keyWire := g.GoType(d, keyField)
				valType, valWire := g.GoType(d, valField)
				keyTag, valTag := g.goTag(d, keyField, keyWire), g.goTag(d, valField, valWire)

				// We don't use stars, except for message-typed values.
				// Message and enum types are the only two possibly foreign types used in maps,
				// so record their use. They are not permitted as map keys.
				keyType = strings.TrimPrefix(keyType, "*")
				switch *valField.Type {
				case descriptor.FieldDescriptorProto_TYPE_ENUM:
					valType = strings.TrimPrefix(valType, "*")
					g.RecordTypeUse(valField.GetTypeName())
				case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
					g.RecordTypeUse(valField.GetTypeName())
				default:
					valType = strings.TrimPrefix(valType, "*")
				}

				typename = fmt.Sprintf("map[%s]%s", keyType, valType)
				mapFieldTypes[field] = typename // record for the getter generation

				tag += fmt.Sprintf(" protobuf_key:%s protobuf_val:%s", keyTag, valTag)
			}
		}

		fieldDeprecated := ""
		if field.GetOptions().GetDeprecated() {
			fieldDeprecated = deprecationComment
		}

		dvalue := g.getterDefault(field, goTypeName)
		if oneof {
			tname := goTypeName + "_" + fieldName
			// It is possible for this to collide with a message or enum
			// nested in this message. Check for collisions.
			for {
				ok := true
				for _, desc := range message.nested {
					if CamelCaseSlice(desc.TypeName()) == tname {
						ok = false
						break
					}
				}
				for _, enum := range message.enums {
					if CamelCaseSlice(enum.TypeName()) == tname {
						ok = false
						break
					}
				}
				if !ok {
					tname += "_"
					continue
				}
				break
			}

			oneofField := oFields[*field.OneofIndex]
			tag := "protobuf:" + g.goTag(message, field, wiretype)
			sf := oneofSubField{
				fieldCommon: fieldCommon{
					goName:     fieldName,
					getterName: fieldGetterName,
					goType:     typename,
					tags:       tag,
					protoName:  field.GetName(),
					fullPath:   fmt.Sprintf("%s,%d,%d", message.path, messageFieldPath, i),
				},
				protoTypeName: field.GetTypeName(),
				fieldNumber:   int(*field.Number),
				protoType:     *field.Type,
				getterDef:     dvalue,
				protoDef:      field.GetDefaultValue(),
				oneofTypeName: tname,
				deprecated:    fieldDeprecated,
			}
			oneofField.subFields = append(oneofField.subFields, &sf)
			g.RecordTypeUse(field.GetTypeName())
			continue
		}

		fieldFullPath := fmt.Sprintf("%s,%d,%d", message.path, messageFieldPath, i)
		c, ok := g.makeComments(fieldFullPath)
		if ok {
			c += "\n"
		}
		rf := simpleField{
			fieldCommon: fieldCommon{
				goName:     fieldName,
				getterName: fieldGetterName,
				goType:     typename,
				tags:       tag,
				protoName:  field.GetName(),
				fullPath:   fieldFullPath,
			},
			protoTypeName: field.GetTypeName(),
			protoType:     *field.Type,
			deprecated:    fieldDeprecated,
			getterDef:     dvalue,
			protoDef:      field.GetDefaultValue(),
			comment:       c,
		}
		var pf topLevelField = &rf

		topLevelFields = append(topLevelFields, pf)
		g.RecordTypeUse(field.GetTypeName())
	}

	mc := &msgCtx{
		goName:  goTypeName,
		message: message,
	}

	g.generateMessageStruct(mc, topLevelFields)
	g.P()
	g.generateCommonMethods(mc)
	g.P()
	g.generateDefaultConstants(mc, topLevelFields)
	g.P()
	g.generateGetters(mc, topLevelFields)
	g.P()
	g.generateSetters(mc, topLevelFields)
	g.P()
	g.generateOneofFuncs(mc, topLevelFields)
	g.P()

	var oneofTypes []string
	for _, f := range topLevelFields {
		if of, ok := f.(*oneofField); ok {
			for _, osf := range of.subFields {
				oneofTypes = append(oneofTypes, osf.oneofTypeName)
			}
		}
	}

	opts := message.Options
	ms := &messageSymbol{
		sym:           goTypeName,
		hasExtensions: len(message.ExtensionRange) > 0,
		isMessageSet:  opts != nil && opts.GetMessageSetWireFormat(),
		oneofTypes:    oneofTypes,
	}
	g.file.addExport(message, ms)

	for _, ext := range message.ext {
		g.generateExtension(ext)
	}

	fullName := strings.Join(message.TypeName(), ".")
	if g.file.Package != nil {
		fullName = *g.file.Package + "." + fullName
	}

	g.addInitf("%s.RegisterType((*%s)(nil), %q)", g.Pkg["proto"], goTypeName, fullName)
	// Register types for native map types.
	for _, k := range mapFieldKeys(mapFieldTypes) {
		fullName := strings.TrimPrefix(*k.TypeName, ".")
		g.addInitf("%s.RegisterMapType((%s)(nil), %q)", g.Pkg["proto"], mapFieldTypes[k], fullName)
	}

}

type byTypeName []*descriptor.FieldDescriptorProto

func (a byTypeName) Len() int           { return len(a) }
func (a byTypeName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byTypeName) Less(i, j int) bool { return *a[i].TypeName < *a[j].TypeName }

// mapFieldKeys returns the keys of m in a consistent order.
func mapFieldKeys(m map[*descriptor.FieldDescriptorProto]string) []*descriptor.FieldDescriptorProto {
	keys := make([]*descriptor.FieldDescriptorProto, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Sort(byTypeName(keys))
	return keys
}

func (g *Generator) generateExtension(ext *ExtensionDescriptor) {
	ccTypeName := ext.DescName()

	extObj := g.ObjectNamed(*ext.Extendee)
	var extDesc *Descriptor
	if id, ok := extObj.(*ImportedDescriptor); ok {
		// This is extending a publicly imported message.
		// We need the underlying type for goTag.
		extDesc = id.o.(*Descriptor)
	} else {
		extDesc = extObj.(*Descriptor)
	}
	extendedType := "*" + g.TypeName(extObj) // always use the original
	field := ext.FieldDescriptorProto
	fieldType, wireType := g.GoType(ext.parent, field)
	tag := g.goTag(extDesc, field, wireType)
	g.RecordTypeUse(*ext.Extendee)
	if n := ext.FieldDescriptorProto.TypeName; n != nil {
		// foreign extension type
		g.RecordTypeUse(*n)
	}

	typeName := ext.TypeName()

	// Special case for proto2 message sets: If this extension is extending
	// proto2.bridge.MessageSet, and its final name component is "message_set_extension",
	// then drop that last component.
	//
	// TODO: This should be implemented in the text formatter rather than the generator.
	// In addition, the situation for when to apply this special case is implemented
	// differently in other languages:
	// https://github.com/google/protobuf/blob/aff10976/src/google/protobuf/text_format.cc#L1560
	mset := false
	if extDesc.GetOptions().GetMessageSetWireFormat() && typeName[len(typeName)-1] == "message_set_extension" {
		typeName = typeName[:len(typeName)-1]
		mset = true
	}

	// For text formatting, the package must be exactly what the .proto file declares,
	// ignoring overrides such as the go_package option, and with no dot/underscore mapping.
	extName := strings.Join(typeName, ".")
	if g.file.Package != nil {
		extName = *g.file.Package + "." + extName
	}

	g.P("var ", ccTypeName, " = &", g.Pkg["proto"], ".ExtensionDesc{")
	g.P("ExtendedType: (", extendedType, ")(nil),")
	g.P("ExtensionType: (", fieldType, ")(nil),")
	g.P("Field: ", field.Number, ",")
	g.P(`Name: "`, extName, `",`)
	g.P("Tag: ", tag, ",")
	g.P(`Filename: "`, g.file.GetName(), `",`)

	g.P("}")
	g.P()

	g.addInitf("%s.RegisterExtension(%s)", g.Pkg["proto"], ext.DescName())
	if mset {
		// Generate a bit more code to register with message_set.go.
		g.addInitf("%s.RegisterMessageSetType((%s)(nil), %d, %q)", g.Pkg["proto"], fieldType, *field.Number, extName)
	}

	g.file.addExport(ext, constOrVarSymbol{ccTypeName, "var", ""})
}

func (g *Generator) generateInitFunction() {
	if len(g.init) == 0 {
		return
	}
	g.P("func init() {")
	for _, l := range g.init {
		g.P(l)
	}
	g.P("}")
	g.init = nil
}

func (g *Generator) generateFileDescriptor(file *FileDescriptor) {
	// Make a copy and trim source_code_info data.
	// TODO: Trim this more when we know exactly what we need.
	pb := proto.Clone(file.FileDescriptorProto).(*descriptor.FileDescriptorProto)
	pb.SourceCodeInfo = nil

	b, err := proto.Marshal(pb)
	if err != nil {
		g.Fail(err.Error())
	}

	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	w.Write(b)
	w.Close()
	b = buf.Bytes()

	v := file.VarName()
	g.P()
	g.P("func init() { ", g.Pkg["proto"], ".RegisterFile(", strconv.Quote(*file.Name), ", ", v, ") }")
	g.P("var ", v, " = []byte{")
	g.P("// ", len(b), " bytes of a gzipped FileDescriptorProto")
	for len(b) > 0 {
		n := 16
		if n > len(b) {
			n = len(b)
		}

		s := ""
		for _, c := range b[:n] {
			s += fmt.Sprintf("0x%02x,", c)
		}
		g.P(s)

		b = b[n:]
	}
	g.P("}")
}

func (g *Generator) generateEnumRegistration(enum *EnumDescriptor) {
	// // We always print the full (proto-world) package name here.
	pkg := enum.File().GetPackage()
	if pkg != "" {
		pkg += "."
	}
	// The full type name
	typeName := enum.TypeName()
	// The full type name, CamelCased.
	ccTypeName := CamelCaseSlice(typeName)
	g.addInitf("%s.RegisterEnum(%q, %[3]s_name, %[3]s_value)", g.Pkg["proto"], pkg+ccTypeName, ccTypeName)
}
