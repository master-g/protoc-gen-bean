package generator

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// And now lots of helper functions.

// Is c an ASCII lower-case letter?
func isASCIILower(c byte) bool {
	return 'a' <= c && c <= 'z'
}

// Is c an ASCII upper-case letter?
func isASCIIUpper(c byte) bool {
	return 'A' <= c && c <= 'Z'
}

// Is c an ASCII digit?
func isASCIIDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// CamelCase returns the CamelCased name.
func CamelCase(s string) string {
	if s == "" {
		return ""
	}
	t := make([]byte, 0, 32)
	i := 0
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
		if isASCIILower(c) {
			c ^= ' ' // Make it a capital letter.
		}
		t = append(t, c) // Guaranteed not lower case.
		// Accept lower case sequence that follows.
		for i+1 < len(s) && isASCIILower(s[i+1]) {
			i++
			t = append(t, s[i])
		}
	}
	if isASCIIUpper(t[0]) {
		t[0] ^= ' '
	}
	return string(t)
}

// CamelCaseSlice is like CamelCase, but the argument is a slice of strings to
// be joined with "_".
func CamelCaseSlice(elem []string) string { return CamelCase(strings.Join(elem, "_")) }

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
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_SINT32,
		descriptor.FieldDescriptorProto_TYPE_SINT64:
		return true
	default:
		return false
	}
}

// paramToJavaPackage convert parameter to a valid java package name
func paramToJavaPackage(param string) string {
	if param == "" {
		return ""
	}

	param = strings.ToLower(param)
	for strings.HasPrefix(param, ".") {
		param = strings.TrimPrefix(param, ".")
	}
	for strings.HasSuffix(param, ".") {
		param = strings.TrimSuffix(param, ".")
	}

	return param
}

// javaType returns a string representing the type name, and the wire type
func javaType(field *descriptor.FieldDescriptorProto) string {
	repeat := isRepeated(field)
	switch *field.Type {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		if repeat {
			return "List<Double>"
		} else {
			return "double"
		}
	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		if repeat {
			return "List<Float>"
		} else {
			return "float"
		}
	case descriptor.FieldDescriptorProto_TYPE_INT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_UINT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SFIXED64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SINT64:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FIXED64:
		if repeat {
			return "List<Long>"
		} else {
			return "long"
		}
	case descriptor.FieldDescriptorProto_TYPE_INT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_UINT32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_FIXED32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SFIXED32:
		fallthrough
	case descriptor.FieldDescriptorProto_TYPE_SINT32:
		if repeat {
			return "List<Integer>"
		} else {
			return "int"
		}
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		if repeat {
			return "List<Boolean>"
		} else {
			return "boolean"
		}
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		if repeat {
			return "List<String>"
		} else {
			return "String"
		}
	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		return "byte[]"
	default:
		return ""
	}
}

func javaFieldName(field *descriptor.FieldDescriptorProto) string {
	return CamelCase(field.GetName())
}

// javaConverterName return java protobuf converter class name
func javaConverterName(file *FileDescriptor) string {
	javaClsName := ""
	if file.GetOptions() != nil && file.GetOptions().GetJavaOuterClassname() != "" {
		javaClsName = file.GetOptions().GetJavaOuterClassname()
	} else {
		javaClsName = file.GetPackage()
		parts := strings.Split(javaClsName, ".")
		javaClsName = parts[len(parts)-1]
	}

	if strings.HasPrefix(strings.ToLower(javaClsName), "pb") {
		javaClsName = javaClsName[2:]
	}
	javaClsName = strings.Title(javaClsName)
	return fmt.Sprintf("%sPb2JavaBean", javaClsName)
}

// badToUnderscore is the mapping function used to generate Go names from package names,
// which can be dotted in the input .proto file.  It replaces non-identifier characters such as
// dot or dash with underscore.
func badToUnderscore(r rune) rune {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
		return r
	}
	return '_'
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

func cleanPackageName(name string) JavaPackageName {
	// name = strings.Map(badToUnderscore, name)
	// // Identifier must not be keyword or predeclared identifier: insert _.
	// if isGoKeyword[name] {
	// 	name = "_" + name
	// }
	// // Identifier must not begin with digit: insert _.
	// if r, _ := utf8.DecodeRuneInString(name); unicode.IsDigit(r) {
	// 	name = "_" + name
	// }
	return JavaPackageName(name)
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
