package generator

import (
	"strings"
)

func getFullPathComponents(g *Generator, f *FileDescriptor, typeName []string) []string {
	p := make([]string, 0)
	if g.ValueObjectPackage != "" {
		p = append(p, strings.Split(g.ValueObjectPackage, ".")...)
	} else if f != nil && f.GetPackage() != "" {
		p = append(p, strings.Split(f.GetPackage(), ".")...)
	}
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
