package generator

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"

	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

const (
	// GeneratorName of this generator
	GeneratorName = "protoc-gen-bean"
	// DefaultIndent for tab or space
	DefaultIndent = "    "
)

// Generator is the type whose methods generate the output, stored in the associated response structure.
type Generator struct {
	*bytes.Buffer

	Request  *plugin.CodeGeneratorRequest  // The input.
	Response *plugin.CodeGeneratorResponse // The output.

	Param map[string]string // Command-line parameters.

	ValueObjectPackage string // Java value object output package
	ConverterPackage   string // Java bean converter output package

	allFiles         []*FileDescriptor          // All files in the tree
	allFilesByName   map[string]*FileDescriptor // All files by input filename.
	genFiles         []*FileDescriptor          // Those files we will generate output for.
	file             *FileDescriptor            // the file we are compiling now.
	typeNameToObject map[string]Object          // Key is a fully-qualified name in input syntax.
	indent           string
	pathType         pathType // How to generate output filenames.
	writeOutput      bool
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
	log.Printf("%s: error:%v", GeneratorName, s)
	os.Exit(1)
}

// Fail reports a problem and exits the program.
func (g *Generator) Fail(msgs ...string) {
	s := strings.Join(msgs, " ")
	log.Printf("%v: error:%v", GeneratorName, s)
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
		case "vopkg":
			g.ValueObjectPackage = paramToJavaPackage(v)
		case "cvtpkg":
			g.ConverterPackage = paramToJavaPackage(v)
		}
	}

	if g.ValueObjectPackage == "" {
		g.Fail("invalid vo package, use --bean_out=vopkg=[package.of.vo], to set")
	}
	if g.ConverterPackage == "" {
		g.ConverterPackage = fmt.Sprintf("%s.%s", g.ValueObjectPackage, "converter")
	}
}

// WrapTypes walks the incoming data, wrapping DescriptorProtos, EnumDescriptorProtos
// and FileDescriptorProtos into file-referenced objects within the Generator.
// It also creates the list of files to generate and so should be called before GenerateAllFiles.
func (g *Generator) WrapTypes() {
	g.allFiles = make([]*FileDescriptor, 0, len(g.Request.ProtoFile))
	g.allFilesByName = make(map[string]*FileDescriptor, len(g.allFiles))

	for _, f := range g.Request.ProtoFile {
		fd := &FileDescriptor{
			FileDescriptorProto: f,
			proto3:              fileIsProto3(f),
		}

		// import path of this file
		fd.importPath = JavaImportPath(strings.Join([]string{
			g.ValueObjectPackage,
			strings.ToLower(f.GetPackage()),
		}, "."))

		// We must wrap the descriptors before we wrap the enums
		fd.desc = wrapDescriptors(fd)
		g.buildNestedDescriptors(fd.desc)
		fd.enum = wrapEnumDescriptors(fd, fd.desc)
		g.buildNestedEnums(fd.desc, fd.enum)
		extractComments(fd)
		g.allFiles = append(g.allFiles, fd)
		g.allFilesByName[f.GetName()] = fd
	}
	for _, fd := range g.allFiles {
		fd.imp = wrapImported(fd, g)
	}

	// output single file for all non-nested descriptors and enums
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
	}
	return
}

func (g *Generator) fileByName(filename string) *FileDescriptor {
	return g.allFilesByName[filename]
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
		g.printAtom(v)
	}
	g.WriteByte('\n')
}

// In Indents the output one tab stop.
func (g *Generator) In() { g.indent += DefaultIndent }

// Out unindents the output one tab stop.
func (g *Generator) Out() {
	if len(g.indent) > 0 {
		g.indent = g.indent[len(DefaultIndent):]
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
	if loc.LeadingComments == nil {
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

func (g *Generator) tailingComments(path string) (string, bool) {
	loc, ok := g.file.comments[path]
	if !ok {
		return "", false
	}
	if loc.TrailingComments == nil {
		return "", false
	}
	w := new(bytes.Buffer)
	nl := ""
	for _, line := range strings.Split(strings.TrimSuffix(loc.GetTrailingComments(), "\n"), "\n") {
		fmt.Fprintf(w, "%s//%s", nl, line)
		nl = "\n"
	}
	return w.String(), true
}

// GenerateAllFiles generates the output for all the files we're outputting.
func (g *Generator) GenerateAllFiles() {
	// Generate the output. The generator runs for every file, even the files
	// that we don't generate output for, so that we can collate the full list
	// of exported symbols to support public imports.
	genFileMap := make(map[*FileDescriptor]bool, len(g.genFiles))
	for _, file := range g.genFiles {
		genFileMap[file] = true
	}
	for _, file := range g.allFiles {
		g.writeOutput = genFileMap[file]
		if !g.writeOutput {
			continue
		}
		g.generateBeans(file)
		g.generateConverters(file)
	}
}

// Fill the response protocol buffer with the generated output for all the descriptors in the file
func (g *Generator) generateBeans(file *FileDescriptor) {
	g.file = file
	// enums
	for _, e := range file.enum {
		if e.parent != nil {
			// nested enum wraps in its parent descriptor
			continue
		}
		g.Reset()

		populateEnum(g, e)

		fullPath := getFullPathComponents(g, file, e.TypeName())
		fullPath = append(fullPath[:len(fullPath)-1], fmt.Sprintf("%s.java", e.GetName()))
		g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(path.Join(fullPath...)),
			Content: proto.String(g.String()),
		})
	}

	// descriptors
	for _, d := range file.desc {
		if d.parent != nil {
			// nested message wraps in its parent descriptor
			continue
		}
		g.Reset()

		populateDescriptor(g, d)

		fullPath := getFullPathComponents(g, file, d.TypeName())
		fullPath = append(fullPath[:len(fullPath)-1], fmt.Sprintf("%s.java", d.GetName()))
		g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
			Name:    proto.String(path.Join(fullPath...)),
			Content: proto.String(g.String()),
		})
	}
}

// Fill the response protocol buffer with the generated output for all the files we're
// supposed to generate.
func (g *Generator) generateConverters(file *FileDescriptor) {
	g.file = file

	javaClsName := javaConverterName(file)

	pathComp := append(strings.Split(g.ConverterPackage, "."), fmt.Sprintf("%s.java", javaClsName))
	g.Reset()
	populatePbToBeanConverter(g, file, javaClsName)
	g.Response.File = append(g.Response.File, &plugin.CodeGeneratorResponse_File{
		Name:    proto.String(path.Join(pathComp...)),
		Content: proto.String(g.String()),
	})
	log.Printf("\n%s", g.String())
}
