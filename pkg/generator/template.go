package generator

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// TODO: add keyword conversion

// deprecationComment is the standard comment added to deprecated
// messages, fields, enums, and enum values.
var deprecationComment = "// Deprecated: Do not use."

func populateHeaderComment(g *Generator, f *FileDescriptor) {
	g.P()
	g.P("// Code generated by ", GeneratorName, ". DO NOT EDIT.")
	g.P("// ", time.Now().Format("2006-01-02 Mon 15:04:05 UTC-0700"))
	g.P("//")
	g.P("//     ", f.GetName())
	g.P("//")
	if f.GetOptions().GetDeprecated() {
		g.P(deprecationComment)
	}
	g.P()
	g.P()
}

func populateEnum(g *Generator, enum *EnumDescriptor) {
	if enum.parent == nil {
		g.P("package ", enumPackagePath(g, enum), ";")
		populateHeaderComment(g, enum.File())
	}

	if enum.GetOptions().GetDeprecated() {
		g.P(deprecationComment)
	}

	g.PrintComments(enum.path)
	g.P("public enum ", enum.GetName(), " {")

	// in order to add default value, need to iterate two rounds
	addDefaultValue := true
	defaultName := "Unknown"
	var defaultValue int32
	defaultValue = -1
	for _, e := range enum.Value {
		low := strings.ToLower(e.GetName())
		if addDefaultValue && // save some string comparision
			(strings.Contains(low, "default") ||
				strings.Contains(low, "unknow") || // the missing 'n' is for poor spelling
				strings.Contains(low, "invalid")) {
			addDefaultValue = false
			defaultName = e.GetName()
			break
		}
		if e.GetNumber() <= defaultValue {
			defaultValue = e.GetNumber() - 1
		}
	}

	g.P()
	g.In()

	if addDefaultValue {
		g.P(defaultName, "(", &defaultValue, "),")
	}
	for i, e := range enum.Value {
		etorPath := fmt.Sprintf("%s,%d,%d", enum.path, enumValuePath, i)
		g.PrintComments(etorPath)

		tails, ok := g.tailingComments(etorPath)
		if !ok {
			tails = ""
		} else {
			tails = fmt.Sprintf(" %s", tails)
		}

		if i == len(enum.Value)-1 {
			g.P(e.GetName(), "(", e.Number, ");", tails)
		} else {
			g.P(e.GetName(), "(", e.Number, "),", tails)
		}
	}
	g.P()
	g.P("public int code;")
	g.P()
	g.P(enum.GetName(), "(int code) { this.code = code; }")
	g.P()
	g.P("/**")
	g.P(" * @deprecated Use {@link #forNumber(int)} instead.")
	g.P(" */")
	g.P("@java.lang.Deprecated")
	g.P("public static ", enum.GetName(), " valueOf(int value) {")
	g.In()
	g.P("return forNumber(value);")
	g.Out()
	g.P("}")
	g.P()
	g.P("public static ", enum.GetName(), " forNumber(int value) {")
	g.In()
	g.P("switch (value) {")
	g.In()
	for _, e := range enum.Value {
		g.P("case ", e.Number, ": return ", e.GetName(), ";")
	}
	g.P("default: return ", defaultName, ";")
	g.Out()
	g.P("}")
	g.Out()
	g.P("}")

	g.Out()
	g.P("}")
}

func getFieldTypeName(g *Generator, field *descriptor.FieldDescriptorProto) string {
	obj, ok := g.typeNameToObject[field.GetTypeName()]
	if !ok {
		g.Fail("unable to find object with type named,", field.GetTypeName())
	}
	// .package.name.TypeName -> package.name.TypeName
	typeName := field.GetTypeName()[1:]
	// package.name.TypeName -> TypeName
	typeName = strings.TrimPrefix(typeName, obj.File().GetPackage())[1:]

	return typeName
}

func extractImports(g *Generator, msg *Descriptor, sysImp, usrImp map[string]string) {
	// nested descriptor's fields
	for _, nested := range msg.nested {
		extractImports(g, nested, sysImp, usrImp)
	}

	if len(msg.Field) != 0 {
		sysImp["java.io.Serializable"] = msg.GetName()
	}

	for _, field := range msg.Field {
		switch field.GetType() {
		case descriptor.FieldDescriptorProto_TYPE_BYTES:
			sysImp["java.util.Arrays"] = field.GetName()
		case descriptor.FieldDescriptorProto_TYPE_ENUM:
			fallthrough
		case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
			if isRepeated(field) {
				sysImp["java.util.List"] = field.GetName()
			}
			obj, ok := g.typeNameToObject[field.GetTypeName()]
			if !ok {
				g.Fail("unable to find object with type named,", field.GetTypeName())
			}
			// .package.name.TypeName -> package.name.TypeName
			typeName := field.GetTypeName()[1:]
			// package.name.TypeName -> TypeName
			typeName = strings.TrimPrefix(typeName, obj.File().GetPackage())[1:]

			// RootMsg.NestMsg -> RootMsg
			importPkg := typeName
			if strings.Index(typeName, ".") != -1 {
				importPkg = strings.Split(typeName, ".")[0]
			}

			fullJavaImportPath := fmt.Sprintf("%s.%s", obj.JavaImportPath().String(), importPkg)
			usrImp[fullJavaImportPath] = typeName
		}
	}
}

func populateField(g *Generator, msg *Descriptor, field *descriptor.FieldDescriptorProto, index int) {
	typeDesc := ""
	switch field.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		typeDesc = getFieldTypeName(g, field)
		if isRepeated(field) {
			typeDesc = fmt.Sprintf("List<%s>", typeDesc)
		}
	default:
		typeDesc = javaType(field)
		if typeDesc == "" {
			g.Fail("unknown type for", field.GetName())
		}
	}

	ftorPath := fmt.Sprintf("%s,%d,%d", msg.path, messageFieldPath, index)
	g.PrintComments(ftorPath)
	tail, ok := g.tailingComments(ftorPath)
	if !ok {
		tail = ""
	} else {
		tail = fmt.Sprintf(" %s", tail)
	}
	g.P("public ", typeDesc, " ", javaFieldName(field), ";", tail)
}

func populateToString(g *Generator, msg *Descriptor) {
	g.P("@Override")
	g.P("public String toString() {")
	g.In()
	g.P("return \"", msg.GetName(), "{\" +")
	g.In()
	g.In()
	sb := &strings.Builder{}
	for i, field := range msg.Field {
		name := javaFieldName(field)

		sb.Reset()
		sb.WriteByte('"')
		if i != 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(name)
		sb.WriteByte('=')
		if field.GetType() == descriptor.FieldDescriptorProto_TYPE_STRING {
			sb.WriteByte('\'')
		}

		sb.WriteString("\" + ")

		if field.GetType() == descriptor.FieldDescriptorProto_TYPE_BYTES {
			sb.WriteString(fmt.Sprintf("Arrays.toString(%s)", name))
		} else {
			sb.WriteString(name)
		}

		if field.GetType() == descriptor.FieldDescriptorProto_TYPE_STRING {
			sb.WriteString(" + '\\''")
		}

		sb.WriteString(" + ")

		g.P(sb.String())
	}
	g.P("\"}\";")
	g.Out()
	g.Out()
	g.Out()
	g.P("}")
}

func populateDescriptor(g *Generator, msg *Descriptor) {
	if msg.parent == nil {
		thisPackage := descriptorPackagePath(g, msg)
		g.P("package ", thisPackage, ";")
		populateHeaderComment(g, msg.File())

		// imports
		sysImp := make(map[string]string)
		usrImp := make(map[string]string)
		extractImports(g, msg, sysImp, usrImp)

		if len(usrImp) > 0 {
			usrImpKeys := make([]string, 0, len(usrImp))
			for importPath := range usrImp {
				usrImpKeys = append(usrImpKeys, importPath)
			}
			sort.Strings(usrImpKeys)
			addParagraph := false
			for _, p := range usrImpKeys {
				if !strings.HasPrefix(p, thisPackage) {
					g.P("import ", p, ";")
					addParagraph = true
				}
			}
			if addParagraph {
				g.P()
			}
		}

		if len(sysImp) > 0 {
			sysImpKeys := make([]string, 0, len(sysImp))
			for importPath := range sysImp {
				sysImpKeys = append(sysImpKeys, importPath)
			}
			sort.Strings(sysImpKeys)
			for _, p := range sysImpKeys {
				g.P("import ", p, ";")
			}
			g.P()
		}
	}

	if msg.GetOptions().GetDeprecated() {
		g.P(deprecationComment)
	}

	g.PrintComments(msg.path)
	serializable := " implements Serializable"
	staticable := " static"
	if len(msg.Field) == 0 {
		serializable = ""
	}
	if msg.parent == nil {
		staticable = ""
	}
	g.P("public", staticable, " final ", msg.GetName(), serializable, " {")
	g.In()
	// fields
	for i, field := range msg.Field {
		populateField(g, msg, field, i)
	}

	// nested enums
	for _, nestEnum := range msg.enums {
		g.P()
		populateEnum(g, nestEnum)
	}

	// nested descriptors
	for _, nestDesc := range msg.nested {
		g.P()
		populateDescriptor(g, nestDesc)
	}
	g.P()
	if len(msg.Field) > 0 {
		populateToString(g, msg)
	}

	g.Out()
	g.P("}")
}