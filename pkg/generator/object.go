package generator

// Object is an interface abstracting the abilities shared by enums, messages, extensions and imported objects.
type Object interface {
	JavaImportPath() JavaImportPath
	TypeName() []string
	File() *FileDescriptor
}
