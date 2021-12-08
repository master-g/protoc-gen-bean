package generator

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

func getFullPathComponents(g *Generator, f *FileDescriptor, typeName []string) []string {
	p := make([]string, 0)
	if g.ValueObjectPackage != "" {
		p = append(p, strings.Split(g.ValueObjectPackage, ".")...)
	}
	// if f != nil && f.GetPackage() != "" {
	// 	p = append(p, strings.Split(f.GetPackage(), ".")...)
	// }
	p = append(p, typeName...)

	return p
}

func descriptorPackagePath(g *Generator, d *Descriptor) string {
	p := getFullPathComponents(g, d.file, d.TypeName())
	if len(p) > 0 {
		p = p[:len(p)-1]
	}
	return strings.Join(p, ".")
}

func descriptorImportPath(g *Generator, d *Descriptor) string {
	p := getFullPathComponents(g, d.file, d.TypeName())
	return strings.Join(p, ".")
}

func enumPackagePath(g *Generator, enum *EnumDescriptor) string {
	p := getFullPathComponents(g, enum.file, enum.TypeName())
	if len(p) > 0 {
		p = p[:len(p)-1]
	}
	return strings.Join(p, ".")
}

func enumImportPath(g *Generator, enum *EnumDescriptor) string {
	p := getFullPathComponents(g, enum.file, enum.TypeName())
	return strings.Join(p, ".")
}

func getOneofName(msg *Descriptor, field *descriptor.FieldDescriptorProto) string {
	if field.OneofIndex != nil {
		odp := msg.OneofDecl[int(*field.OneofIndex)]
		return CamelCase(odp.GetName())
	} else {
		return ""
	}
}

type oneofSubField struct {
	index int
	field *descriptor.FieldDescriptorProto
}

func (f *oneofSubField) getEnumName() string {
	return strings.ToUpper(f.field.GetName())
}

type oneofField struct {
	name      string
	field     *descriptor.FieldDescriptorProto
	subFields []*oneofSubField
}

func (f *oneofField) getCaseClassName() string {
	return fmt.Sprintf("%vCase", strings.Title(f.name))
}

func (f *oneofField) populate(g *Generator, d *Descriptor) {
	if len(f.subFields) == 0 {
		return
	}

	g.P("enum class ", f.getCaseClassName(), "(val code: Int) {")
	g.In()
	for _, sf := range f.subFields {
		g.P(sf.getEnumName(), "(", sf.field.Number, "),")
	}
	g.P(strings.ToUpper(f.name), "_NOT_SET(0);")
	// companion
	g.Newline()
	g.P("companion object {")
	g.In()
	g.P("fun forNumber(value: Int): ", f.getCaseClassName(), " {")
	g.In()
	g.P("return when (value) {")
	g.In()
	for _, sf := range f.subFields {
		g.P(sf.getEnumName(), ".code -> ", sf.getEnumName())
	}
	notSet := fmt.Sprintf("%v_%v", strings.ToUpper(f.name), "NOT_SET")
	g.P("else -> ", notSet)
	g.Out()
	g.P("}")
	g.Out()
	g.P("}")
	g.Out()
	g.P("}")

	g.Out()
	g.P("}")
	g.Newline()
	g.P("var ", f.name, "Case: ", f.getCaseClassName(), " = ", f.getCaseClassName(), ".", notSet)
}
