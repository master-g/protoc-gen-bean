package generator

// symbol is an interface representing an exported Go symbol.
type symbol interface {
	// GenerateAlias should generate an appropriate alias
	// for the symbol from the named package.
	GenerateAlias(g *Generator, filename string, pkg GoPackageName)
}

type messageSymbol struct {
	sym                         string
	hasExtensions, isMessageSet bool
	oneofTypes                  []string
}

type getterSymbol struct {
	name     string
	typ      string
	typeName string // canonical name in proto world; empty for proto.Message and similar
	genType  bool   // whether typ contains a generated type (message/group/enum)
}

func (ms *messageSymbol) GenerateAlias(g *Generator, filename string, pkg GoPackageName) {
	g.P("// ", ms.sym, " from public import ", filename)
	g.P("type ", ms.sym, " = ", pkg, ".", ms.sym)
	for _, name := range ms.oneofTypes {
		g.P("type ", name, " = ", pkg, ".", name)
	}
}

type enumSymbol struct {
	name   string
	proto3 bool // Whether this came from a proto3 file.
}

func (es enumSymbol) GenerateAlias(g *Generator, filename string, pkg GoPackageName) {
	s := es.name
	g.P("// ", s, " from public import ", filename)
	g.P("type ", s, " = ", pkg, ".", s)
	g.P("var ", s, "_name = ", pkg, ".", s, "_name")
	g.P("var ", s, "_value = ", pkg, ".", s, "_value")
}

type constOrVarSymbol struct {
	sym  string
	typ  string // either "const" or "var"
	cast string // if non-empty, a type cast is required (used for enums)
}

func (cs constOrVarSymbol) GenerateAlias(g *Generator, filename string, pkg GoPackageName) {
	v := string(pkg) + "." + cs.sym
	if cs.cast != "" {
		v = cs.cast + "(" + v + ")"
	}
	g.P(cs.typ, " ", cs.sym, " = ", v)
}
