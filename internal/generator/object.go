package generator

// Object is an interface abstracting the abilities shared by enums, messages, extensions and imported objects.
type Object interface {
	GoImportPath() GoImportPath
	TypeName() []string
	File() *FileDescriptor
}
