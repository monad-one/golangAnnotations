package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MarcGrol/golangAnnotations/model"
)

var (
	debugAstOfSources = false
)

func ParseSourceFile(srcFilename string) (model.ParsedSources, error) {
	if debugAstOfSources {
		dumpFile(srcFilename)
	}
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, srcFilename, nil, parser.ParseComments)
	if err != nil {
		log.Printf("error parsing src %s: %s", srcFilename, err.Error())
		return model.ParsedSources{}, err
	}
	v := &astVisitor{
		Imports: map[string]string{},
	}
	v.CurrentFilename = srcFilename
	ast.Walk(v, file)

	embedOperationsInStructs(v)

	embedTypedefDocLinesInEnum(v)

	result := model.ParsedSources{
		Structs:    v.Structs,
		Operations: v.Operations,
		Interfaces: v.Interfaces,
		Typedefs:   v.Typedefs,
		Enums:      v.Enums,
	}
	return result, nil
}

type FileEntry struct {
	key  string
	file ast.File
}

type FileEntries []FileEntry

func (list FileEntries) Len() int {
	return len(list)
}

func (list FileEntries) Less(i, j int) bool {
	return list[i].key < list[j].key
}

func (list FileEntries) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func SortedFileEntries(fileMap map[string]*ast.File) FileEntries {
	var fileEntries FileEntries = make([]FileEntry, 0, len(fileMap))
	for key, file := range fileMap {
		if file != nil {
			fileEntries = append(fileEntries, FileEntry{
				key:  key,
				file: *file,
			})
		}
	}
	sort.Sort(fileEntries)
	return fileEntries
}

func ParseSourceDir(dirName string, filenameRegex string) (model.ParsedSources, error) {
	if debugAstOfSources {
		dumpFilesInDir(dirName)
	}
	packages, err := parseDir(dirName, filenameRegex)
	if err != nil {
		log.Printf("error parsing dir %s: %s", dirName, err.Error())
		return model.ParsedSources{}, err
	}

	v := &astVisitor{
		Imports: map[string]string{},
	}
	for _, aPackage := range packages {
		for _, fileEntry := range SortedFileEntries(aPackage.Files) {
			v.CurrentFilename = fileEntry.key

			appEngineOnly := true
			for _, commentGroup := range fileEntry.file.Comments {
				if commentGroup != nil {
					for _, comment := range commentGroup.List {
						if comment != nil && comment.Text == "// +build !appengine" {
							appEngineOnly = false
						}
					}
				}
			}
			if appEngineOnly {
				ast.Walk(v, &fileEntry.file)
			}
		}
	}

	embedOperationsInStructs(v)

	embedTypedefDocLinesInEnum(v)

	result := model.ParsedSources{
		Structs:    v.Structs,
		Operations: v.Operations,
		Interfaces: v.Interfaces,
		Typedefs:   v.Typedefs,
		Enums:      v.Enums,
	}

	return result, nil
}

func embedOperationsInStructs(visitor *astVisitor) {
	structMap := make(map[string]*model.Struct)
	for idx := range visitor.Structs {
		structMap[(&visitor.Structs[idx]).Name] = &visitor.Structs[idx]
	}
	for idx := range visitor.Operations {
		operation := visitor.Operations[idx]
		if operation.RelatedStruct != nil {
			mStruct, ok := structMap[(*operation.RelatedStruct).TypeName]
			if ok {
				mStruct.Operations = append(mStruct.Operations, &operation)
			}
		}
	}

}

func embedTypedefDocLinesInEnum(v *astVisitor) {
	for idx, mEnum := range v.Enums {
		for _, typedef := range v.Typedefs {
			if typedef.Name == mEnum.Name {
				v.Enums[idx].DocLines = typedef.DocLines
				break
			}
		}
	}
}

func parseDir(dirName string, filenameRegex string) (map[string]*ast.Package, error) {
	var pattern = regexp.MustCompile(filenameRegex)

	packageMap := make(map[string]*ast.Package)
	var err error

	fileSet := token.NewFileSet()
	packageMap, err = parser.ParseDir(
		fileSet,
		dirName,
		func(fi os.FileInfo) bool {
			return pattern.MatchString(fi.Name())
		},
		parser.ParseComments)
	if err != nil {
		log.Printf("error parsing dir %s: %s", dirName, err.Error())
		return packageMap, err
	}

	return packageMap, nil
}

func dumpFile(srcFilename string) {
	fileSet := token.NewFileSet()
	aFile, err := parser.ParseFile(fileSet, srcFilename, nil, parser.ParseComments)
	if err != nil {
		log.Printf("error parsing src %s: %s", srcFilename, err.Error())
		return
	}
	ast.Print(fileSet, aFile)
}

func dumpFilesInDir(dirName string) {
	fileSet := token.NewFileSet()
	packageMap, err := parser.ParseDir(
		fileSet,
		dirName,
		nil,
		parser.ParseComments)
	if err != nil {
		log.Printf("error parsing dir %s: %s", dirName, err.Error())
	}
	for _, aPackage := range packageMap {
		for _, aFile := range aPackage.Files {
			ast.Print(fileSet, aFile)
		}
	}
}

type astVisitor struct {
	CurrentFilename string
	PackageName     string
	Filename        string
	Imports         map[string]string
	Structs         []model.Struct
	Operations      []model.Operation
	Interfaces      []model.Interface
	Typedefs        []model.Typedef
	Enums           []model.Enum
}

func (v *astVisitor) Visit(node ast.Node) ast.Visitor {
	if node != nil {

		// package-name is in isolated node
		packageName, ok := extractPackageName(node)
		if ok {
			v.PackageName = packageName
		}

		// extract all imports into a map
		v.extractGenDeclImports(node)

		{
			// if struct, get its fields
			mStruct, ok := extractGenDeclForStruct(node, v.Imports)
			if ok {
				mStruct.PackageName = v.PackageName
				mStruct.Filename = v.CurrentFilename
				v.Structs = append(v.Structs, mStruct)
			}
		}
		{
			// if struct, get its fields
			mTypedef, ok := extractGenDeclForTypedef(node, v.Imports)
			if ok {
				mTypedef.PackageName = v.PackageName
				mTypedef.Filename = v.CurrentFilename
				v.Typedefs = append(v.Typedefs, mTypedef)
			}
		}
		{
			// if struct, get its fields
			mEnum, ok := extractGenDeclForEnum(node, v.Imports)
			if ok {
				mEnum.PackageName = v.PackageName
				mEnum.Filename = v.CurrentFilename
				v.Enums = append(v.Enums, mEnum)
			}
		}
		{
			// if interfaces, get its methods
			mInterface, ok := extractGenDecForInterface(node, v.Imports)
			if ok {
				mInterface.PackageName = v.PackageName
				mInterface.Filename = v.CurrentFilename
				v.Interfaces = append(v.Interfaces, mInterface)
			}
		}
		{
			// if mOperation, get its signature
			mOperation, ok := extractOperation(node, v.Imports)
			if ok {
				mOperation.PackageName = v.PackageName
				mOperation.Filename = v.CurrentFilename
				v.Operations = append(v.Operations, mOperation)
			}
		}
	}
	return v
}

func (v *astVisitor) extractGenDeclImports(node ast.Node) {
	genDecl, ok := node.(*ast.GenDecl)
	if ok {
		for _, spec := range genDecl.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if ok {
				quotedImport := importSpec.Path.Value
				unquotedImport := strings.Trim(quotedImport, "\"")
				first, last := filepath.Split(unquotedImport)
				if first == "" {
					last = first
				}
				v.Imports[last] = unquotedImport
				//log.Printf( "Found import %s -> %s",  last, unquotedImport)
			}
		}
	}
}

func extractGenDeclForStruct(node ast.Node, imports map[string]string) (model.Struct, bool) {
	found := false
	var mStruct model.Struct

	genDecl, ok := node.(*ast.GenDecl)
	if ok {
		// Continue parsing to see if it a struct
		mStruct, found = extractSpecsForStruct(genDecl.Specs, imports)
		if ok {
			// Docline of struct (that could contain annotations) appear far before the details of the struct
			mStruct.DocLines = extractComments(genDecl.Doc)
		}
	}

	return mStruct, found
}

func extractGenDeclForTypedef(node ast.Node, imports map[string]string) (model.Typedef, bool) {
	found := false
	var mTypedef model.Typedef

	genDecl, ok := node.(*ast.GenDecl)
	if ok {
		// Continue parsing to see if it a struct
		mTypedef, found = extractSpecsForTypedef(genDecl.Specs, imports)
		if found {
			mTypedef.DocLines = extractComments(genDecl.Doc)
		}
	}

	return mTypedef, found
}

func extractGenDeclForEnum(node ast.Node, imports map[string]string) (model.Enum, bool) {
	found := false
	var mEnum model.Enum

	genDecl, ok := node.(*ast.GenDecl)
	if ok {
		// Continue parsing to see if it an enum
		mEnum, found = extractSpecsForEnum(genDecl.Specs, imports)
		// Docs live in the related typdef
	}

	return mEnum, found
}

func extractGenDecForInterface(node ast.Node, imports map[string]string) (model.Interface, bool) {
	found := false
	var mInterface model.Interface

	genDecl, ok := node.(*ast.GenDecl)
	if ok {
		// Continue parsing to see if it an interface
		mInterface, found = extractSpecsForInterface(genDecl.Specs, imports)
		if found {
			// Docline of interface (that could contain annotations) appear far before the details of the struct
			mInterface.DocLines = extractComments(genDecl.Doc)
		}
	}

	return mInterface, found
}

func extractSpecsForStruct(specs []ast.Spec, imports map[string]string) (model.Struct, bool) {
	found := false
	mStruct := model.Struct{}

	if len(specs) >= 1 {
		typeSpec, ok := specs[0].(*ast.TypeSpec)
		if ok {
			mStruct.Name = typeSpec.Name.Name

			structType, ok := typeSpec.Type.(*ast.StructType)
			if ok {
				mStruct.Fields = extractFieldList(structType.Fields, imports)
				found = true
			}
		}
	}

	return mStruct, found
}

func extractSpecsForEnum(specs []ast.Spec, imports map[string]string) (model.Enum, bool) {
	found := false
	mEnum := model.Enum{}

	// parse type part

	// parse const part
	if len(specs) >= 1 {
		isEnumConstant := false
		typeName := ""
		for _, spec := range specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if ok {
				if valueSpec.Type != nil {
					for _, name := range valueSpec.Names {
						ident, ok := valueSpec.Type.(*ast.Ident)
						if ok {
							typeName = ident.Name
						}
						if name.Obj.Kind == ast.Con {
							isEnumConstant = true
							break
						}
					}
				}
			}
		}

		if isEnumConstant {

			mEnum.Name = typeName
			mEnum.EnumLiterals = []model.EnumLiteral{}
			for _, spec := range specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if ok {
					literal := model.EnumLiteral{
						Name: valueSpec.Names[0].Name,
					}

					for _, value := range valueSpec.Values {

						basicLit, ok := value.(*ast.BasicLit)
						if ok {
							literal.Value = strings.Trim(basicLit.Value, "\"")
							break
						}
					}
					mEnum.EnumLiterals = append(mEnum.EnumLiterals, literal)
				}
			}
			found = true
		}
	}

	return mEnum, found
}

func extractSpecsForInterface(specs []ast.Spec, imports map[string]string) (model.Interface, bool) {
	found := false
	mInterface := model.Interface{}

	if len(specs) >= 1 {
		typeSpec, ok := specs[0].(*ast.TypeSpec)
		if ok {
			mInterface.Name = typeSpec.Name.Name

			interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
			if ok {
				mInterface.Methods = extractInterfaceMethods(interfaceType.Methods, imports)
				found = true
			}
		}
	}

	return mInterface, found
}

func extractPackageName(node ast.Node) (string, bool) {
	found := false
	packageName := ""

	file, found := node.(*ast.File)
	if found {
		if file.Name != nil {
			packageName = file.Name.Name
		}
	}
	return packageName, found
}

func extractOperation(node ast.Node, imports map[string]string) (model.Operation, bool) {
	found := false
	mOperation := model.Operation{}

	funcDecl, found := node.(*ast.FuncDecl)
	if found {
		mOperation.DocLines = extractComments(funcDecl.Doc)

		if funcDecl.Recv != nil {
			fields := extractFieldList(funcDecl.Recv, imports)
			if len(fields) >= 1 {
				mOperation.RelatedStruct = &(fields[0])
			}
		}

		if funcDecl.Name != nil {
			mOperation.Name = funcDecl.Name.Name
		}

		if funcDecl.Type.Params != nil {
			mOperation.InputArgs = extractFieldList(funcDecl.Type.Params, imports)
		}

		if funcDecl.Type.Results != nil {
			mOperation.OutputArgs = extractFieldList(funcDecl.Type.Results, imports)
		}
	}
	return mOperation, found
}

func extractSpecsForTypedef(specs []ast.Spec, imports map[string]string) (model.Typedef, bool) {
	found := false
	mTypedef := model.Typedef{}

	if len(specs) >= 1 {
		typeSpec, ok := specs[0].(*ast.TypeSpec)
		if ok {
			mTypedef.Name = typeSpec.Name.Name
			ident, ok := typeSpec.Type.(*ast.Ident)
			if ok {
				mTypedef.Type = ident.Name
			}
			found = true
		}
	}

	return mTypedef, found
}

func extractComments(commentGroup *ast.CommentGroup) []string {
	lines := []string{}
	if commentGroup != nil {
		for _, comment := range commentGroup.List {
			lines = append(lines, comment.Text)
		}
	}
	return lines
}

func extractTag(basicLit *ast.BasicLit) (string, bool) {
	if basicLit != nil {
		return basicLit.Value, true
	}
	return "", false
}

func extractFieldList(fieldList *ast.FieldList, imports map[string]string) []model.Field {
	fields := []model.Field{}
	if fieldList != nil {
		for _, field := range fieldList.List {
			fields = append(fields, extractFields(field, imports)...)
		}
	}
	return fields
}

func extractInterfaceMethods(fieldList *ast.FieldList, imports map[string]string) []model.Operation {
	methods := []model.Operation{}

	for _, field := range fieldList.List {
		if len(field.Names) > 0 {
			mOperation := model.Operation{DocLines: extractComments(field.Doc)}

			mOperation.Name = field.Names[0].Name

			funcType, ok := field.Type.(*ast.FuncType)
			if ok {
				if funcType.Params != nil {
					mOperation.InputArgs = extractFieldList(funcType.Params, imports)
				}

				if funcType.Results != nil {
					mOperation.OutputArgs = extractFieldList(funcType.Results, imports)
				}
				methods = append(methods, mOperation)
			}
		}
	}
	return methods
}

func extractFields(field *ast.Field, imports map[string]string) []model.Field {
	fields := []model.Field{}
	if field != nil {
		if len(field.Names) == 0 {
			fields = append(fields, _extractField(field, imports))
		} else {
			// A single field can refer to multiple: example: x,y int -> x int, y int
			for _, name := range field.Names {
				field := _extractField(field, imports)
				field.Name = name.Name
				fields = append(fields, field)
			}
		}
	}
	return fields
}

func _extractField(input *ast.Field, imports map[string]string) model.Field {
	field := model.Field{}

	field.DocLines = extractComments(input.Doc)

	field.CommentLines = extractComments(input.Comment)

	tag, ok := extractTag(input.Tag)
	if ok {
		field.Tag = tag
	}
	{
		arrayType, ok := input.Type.(*ast.ArrayType)
		if ok {
			field.IsSlice = true
			{
				ident, ok := arrayType.Elt.(*ast.Ident)
				if ok {
					field.TypeName = ident.Name
				}
				selectorExpr, ok := arrayType.Elt.(*ast.SelectorExpr)
				if ok {
					ident, ok = selectorExpr.X.(*ast.Ident)
					if ok {
						field.TypeName = fmt.Sprintf("%s.%s", ident.Name, selectorExpr.Sel.Name)
						field.PackageName = imports[ident.Name]
					}
				}
			}

			{
				starExpr, ok := arrayType.Elt.(*ast.StarExpr)
				if ok {
					if ok {
						ident, ok := starExpr.X.(*ast.Ident)
						if ok {
							field.TypeName = ident.Name
							field.IsPointer = true
						}
					}

					selectorExpr, ok := starExpr.X.(*ast.SelectorExpr)
					if ok {
						ident, ok := selectorExpr.X.(*ast.Ident)
						if ok {
							field.PackageName = imports[ident.Name]
							field.IsPointer = true
							field.TypeName = fmt.Sprintf("%s.%s", ident.Name, selectorExpr.Sel.Name)
						}
					}
				}
			}
		}
	}

	{
		var mapKey string = ""
		var mapValue string = ""

		mapType, ok := input.Type.(*ast.MapType)
		if ok {
			{
				key, ok := mapType.Key.(*ast.Ident)
				if ok {
					mapKey = key.Name
				}
			}
			{
				value, ok := mapType.Value.(*ast.Ident)
				if ok {
					mapValue = value.Name
				}
			}
		}
		if mapKey != "" && mapValue != "" {
			field.TypeName = fmt.Sprintf("map[%s]%s", mapKey, mapValue)
		}

	}

	{
		starExpr, ok := input.Type.(*ast.StarExpr)
		if ok {
			ident, ok := starExpr.X.(*ast.Ident)
			if ok {
				//log.Printf("starExpr ident: %+v", ident.Name)
				field.TypeName = ident.Name
				field.IsPointer = true
			}
			selectorExpr, ok := starExpr.X.(*ast.SelectorExpr)
			if ok {
				ident, ok = selectorExpr.X.(*ast.Ident)
				if ok {
					field.TypeName = fmt.Sprintf("%s.%s", ident.Name, selectorExpr.Sel.Name)
					field.IsPointer = true
					field.PackageName = imports[ident.Name]
				}
			}
		}
	}
	{
		ident, ok := input.Type.(*ast.Ident)
		if ok {
			field.TypeName = ident.Name
		}
	}
	{
		selectorExpr, ok := input.Type.(*ast.SelectorExpr)
		if ok {
			ident, ok := selectorExpr.X.(*ast.Ident)
			if ok {
				field.Name = ident.Name
				field.TypeName = fmt.Sprintf("%s.%s", ident.Name, selectorExpr.Sel.Name)
				field.PackageName = imports[ident.Name]
			}
		}
	}

	return field
}
