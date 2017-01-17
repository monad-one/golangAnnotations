package model

type ParsedSources struct {
	Structs    []Struct
	Operations []Operation
	Interfaces []Interface
	Typedefs   []Typedef
	Enums      []Enum
}

type Operation struct {
	PackageName   string
	Filename      string
	DocLines      []string
	RelatedStruct *Field // optional
	Name          string
	InputArgs     []Field
	OutputArgs    []Field
	CommentLines  []string
}

type Struct struct {
	PackageName  string
	Filename     string
	DocLines     []string
	Name         string
	Fields       []Field
	Operations   []*Operation
	CommentLines []string
}

type Interface struct {
	PackageName  string
	Filename     string
	DocLines     []string
	Name         string
	Methods      []Operation
	CommentLines []string
}

type Field struct {
	PackageName  string
	DocLines     []string
	Name         string
	TypeName     string
	IsSlice      bool
	IsPointer    bool
	Tag          string
	CommentLines []string
}

type Typedef struct {
	PackageName string
	Filename    string
	DocLines    []string
	Name        string
	Type        string
}

type Enum struct {
	PackageName  string
	Filename     string
	DocLines     []string
	Name         string
	EnumLiterals []EnumLiteral
	CommentLines []string
}

type EnumLiteral struct {
	Name  string
	Value string
}
