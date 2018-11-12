package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

func extractConverterImports(g *Generator, f *FileDescriptor, sysImp, usrImp map[string]bool) {
	// always have this
	usrImp["com.google.protobuf.InvalidProtocolBufferException"] = true

	if f.GetOptions() != nil {
		javaPkg := f.GetOptions().GetJavaPackage()
		javaOutClass := f.GetOptions().GetJavaOuterClassname()
		usrImp[fmt.Sprintf("%s.%s", javaPkg, javaOutClass)] = true
	}

	for _, desc := range f.desc {
		typeName := desc.TypeName()
		if len(typeName) > 1 {
			typeName = []string{typeName[0]}
		}
		p := getFullPathComponents(g, desc.file, typeName)
		usrImp[strings.Join(p, ".")] = true

		for _, field := range desc.Field {
			if isRepeated(field) {
				sysImp["java.util.ArrayList"] = true
				sysImp["java.util.List"] = true
				break
			}
			// descriptors can be convert from bytes to object directly
			// since the converter are in the same package
			// there is no need to import the corresponding bean class to the descriptor
			// for example, say 'Foo' has a field type 'Bar'
			// 		Foo entity = new Foo();
			// 		Foo.bar = SomeConverter.toBar(pbEntity.getBar().toByteArray());
			// so, we don't have to import 'com.package.to.value.object.of.Bar'
			if field.GetType() == descriptor.FieldDescriptorProto_TYPE_ENUM {
				// however, enum are little tricky, since we have to access enum value object's 'forNumber' method
				enumTypeName := field.GetTypeName()
				enumObj, ok := g.typeNameToObject[enumTypeName]
				if !ok {
					g.Fail("cannot find object to type,", enumTypeName)
				}
				rootTypeName := enumObj.TypeName()
				if len(rootTypeName) > 1 {
					rootTypeName = []string{rootTypeName[0]}
				}
				objPath := getFullPathComponents(g, enumObj.File(), rootTypeName)
				usrImp[strings.Join(objPath, ".")] = true
			}
		}
	}
}

func populateDescriptorConverter(g *Generator, desc *Descriptor, thisConverter string) {
	typeName := strings.Join(desc.TypeName(), ".")
	funcName := strings.Join(desc.TypeName(), "")
	_, pbClass, ok := desc.file.javaPackageOption()
	if !ok {
		g.Fail("cannot find java output class for,", desc.GetName(), " in,", desc.File().GetName())
	}
	varName := CamelCase(strings.Join(desc.TypeName(), ""))

	g.P("public static ", typeName, " to", funcName, "([]byte data) {")
	g.In()
	g.P("try {")
	g.In()
	g.P(pbClass, ".", typeName, " ", varName, " = ", pbClass, ".", typeName, ".parseFrom(data);")
	g.P(typeName, " entity = new ", typeName, "();")

	for _, field := range desc.Field {
		fieldLowerCamelName := CamelCase(field.GetName())
		fieldUpperCamelCaseName := strings.Title(CamelCase(field.GetName()))
		if field.GetType() == descriptor.FieldDescriptorProto_TYPE_ENUM {
			// enum
			typeObj, ok := g.typeNameToObject[field.GetTypeName()]
			if !ok {
				g.Fail("unable to find object for type,", field.GetTypeName())
			}
			enumClass := strings.Join(typeObj.TypeName(), ".")
			if !isRepeated(field) {
				g.P("entity.", fieldLowerCamelName, " = ", enumClass, ".forNumber(", varName, ".get", strings.Title(fieldLowerCamelName), "Value());")
			} else {
				g.P("entity.", fieldLowerCamelName, " = new ArrayList<>();")
				g.P("for (int value : ", varName, ".get", fieldUpperCamelCaseName, "ValueList()) {")
				g.In()
				g.P("entity.", fieldLowerCamelName, ".add(", enumClass, ".forNumber(value);")
				g.Out()
				g.P("}")
			}
		} else if field.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
			// message
			typeObj, ok := g.typeNameToObject[field.GetTypeName()]
			if !ok {
				g.Fail("unable to find object for type,", field.GetTypeName())
			}
			fieldConverter := javaConverterName(typeObj.File())

			if strings.Compare(fieldConverter, thisConverter) == 0 {
				fieldConverter = ""
			} else {
				fieldConverter = fmt.Sprintf("%s.", fieldConverter)
			}
			fieldConvertMethod := strings.Join(typeObj.TypeName(), "")
			byteSource := fmt.Sprintf("%s.get%s().toByteArray()", varName, fieldUpperCamelCaseName)
			if !isRepeated(field) {
				g.P("if (", varName, ".get", fieldUpperCamelCaseName, "() != null) {")
				g.In()
				g.P("entity.", fieldLowerCamelName, " = ", fieldConverter, "to", fieldConvertMethod, "(", byteSource, ");")
				g.Out()
				g.P("}")
			} else {
				fieldPbClass := []string{typeObj.File().GetOptions().GetJavaOuterClassname()}
				fieldPbClass = append(fieldPbClass, typeObj.TypeName()...)
				fieldPbClassName := strings.Join(fieldPbClass, ".")

				g.P("entity.", fieldLowerCamelName, " = new ArrayList<>();")
				g.P("if (", varName, ".get", fieldUpperCamelCaseName, "Count() > 0) {")
				g.In()
				g.P("for (", fieldPbClassName, " element : ", varName, ".get", fieldUpperCamelCaseName, "List()) {")
				g.In()
				g.P("entity.", fieldLowerCamelName, ".add(", fieldConverter, "to", fieldConvertMethod, "(element.toByteArray());")
				g.Out()
				g.P("}")
				g.Out()
				g.P("}")
			}
		} else {
			g.P("entity.", fieldLowerCamelName, " = ", varName, ".get", fieldUpperCamelCaseName, "();")
		}
	}

	g.P()
	g.P("return entity;")
	g.Out()
	g.P("} catch (InvalidProtocolBufferException e) {")
	g.In()
	g.P("e.printStackTrace();")
	g.P("// SocketLog.e(e);")
	g.Out()
	g.P("}")
	g.P()
	g.P("return null;")
	g.Out()
	g.P("}")
}

func populatePbToBeanConverter(g *Generator, file *FileDescriptor, javaClsName string) {
	g.P("package ", g.ConverterPackage, ";")
	populateHeaderComment(g, file)

	sysImp := make(map[string]bool)
	usrImp := make(map[string]bool)
	extractConverterImports(g, file, sysImp, usrImp)

	keys := make([]string, 0, len(usrImp))
	for k := range usrImp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, p := range keys {
		g.P("import ", p, ";")
	}

	if len(keys) > 0 {
		g.P()
	}

	keys = make([]string, 0, len(sysImp))
	for k := range sysImp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, p := range keys {
		g.P("import ", p, ";")
	}
	if len(keys) > 0 {
		g.P()
	}

	g.P("public static final ", javaClsName, " {")
	g.In()

	for _, desc := range file.desc {
		for _, nested := range desc.nested {
			g.P()
			populateDescriptorConverter(g, nested, javaClsName)
		}
		g.P()
		if desc.parent == nil {
			populateDescriptorConverter(g, desc, javaClsName)
		}
	}

	g.Out()
	g.P("}")
	g.P()
}
