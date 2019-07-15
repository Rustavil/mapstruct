// The following directive is necessary to make the package coherent:

// +bкuild ignore

// This program generates mappers.go. It can be invoked by running
// go generate
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	pathUtil "path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	defaultOutFile = "mappers.go"
)

const (
	mapperTmpl = `// Code generated by go generate; DO NOT EDIT.
// This file was generated by robots at
// {{ .Timestamp }}
// using data from
// {{ .ConfPath }}
package {{ .PackageName }}

import (
{{- range .ImportPackages }}
	{{if .Alias}}{{ .Alias }} {{end}}"{{ .Path }}"
{{- end }}
)
{{- range .TypeToPtrList }}
func {{ . }}Ptr(src {{ . }}) *{{ . }} {
	return &src
}
{{- end }}
{{- range .TypesCastList }}
{{ $name := .Name }}
{{- range .CastTypes }}
func {{ $name }}ArrTo{{ . | ToTitle }}PtrArr(src []{{ $name }}) (dst []*{{ . }}) {
	dst = make([]*{{ . }}, len(src))
	for i := range src {
		dst[i] = {{ . }}Ptr({{ . }}(src[i]))
	}
	return dst
}
func {{ $name }}PtrArrTo{{ . | ToTitle }}Arr(src []*{{ $name }}) (dst []{{ . }}) {
	dst = make([]{{ . }}, len(src))
	for i := range src {
		dst[i] = {{ . }}(*src[i])
	}
	return dst
}
{{- end }}
{{- end }}
{{- range .Mappers }}
{{- $mapper := . }}
func {{ .MapperFuncName }}({{- range  $index, $element := .SrcList }}{{if $index}}, {{end}}{{ $element.Alias }}  *{{ $element.ShortPath }}{{- end }}) ({{ .Dst.Alias }} *{{ .Dst.ShortPath }}) {
	{{- $dst := .Dst }}
	{{- range .SrcList }}
	if {{ .Alias }} != nil {
		if {{ $dst.Alias }} == nil {
			{{ $dst.Alias }} = &{{ $dst.ShortPath }}{}
		}
		{{- range $mapper.FieldMappingRules }}
		{{- if .Casted }}
		{{- if .SrcFieldPtr }}
		if {{ .SrcAlias }}.{{ .SrcFieldName }} != nil {
			{{ $dst.Alias }}.{{ .DstFieldName }} = {{ .CastStr }}
		}
		{{- else }}
		{{ $dst.Alias }}.{{ .DstFieldName }} = {{ .CastStr }}
		{{- end }}
		{{- else }}
		//{{ $dst.Alias }}.{{ .DstFieldName }} = {{ .CastStr }}
		{{- end }}
		{{- end }}
	}
	{{- end }}
	return {{ .Dst.Alias }}
}
func {{ .ListMapperFuncName }}({{- range  $index, $element := .SrcList }}{{if $index}}, {{end}}{{ $element.Alias }}  []*{{ $element.ShortPath }}{{- end }}) ({{ .Dst.Alias }} []*{{ .Dst.ShortPath }}) {
	var count int
	{{- range .SrcList }}
	if count == 0 || count > len({{ .Alias }}) {
		count = len({{ .Alias }})
	}
	{{- end }}
	{{ .Dst.Alias }} = make([]*{{ .Dst.ShortPath }}, 0, count)
	for i := 0; i < count; i++ {
    	{{ .Dst.Alias }} = append(dst, {{ .MapperFuncName }}({{- range $index, $element := .SrcList }}{{if $index}}, {{end}}{{ $element.Alias }}[i]{{- end }}))
    }
	return {{ .Dst.Alias }}
}
{{- end }}`
)

var (
	importPackageAliasMap = make(map[string]string, 10)
	typeToPtrList         = []string{"bool", "string", "byte", "int", "int64", "float32", "float64"}
	typesCastList         = []struct {
		Name      string
		CastTypes []string
	}{
		{
			Name:      "bool",
			CastTypes: []string{"bool"},
		},
		{
			Name:      "string",
			CastTypes: []string{"string"},
		},
		{
			Name:      "int",
			CastTypes: []string{"int", "int64"},
		},
		{
			Name:      "int64",
			CastTypes: []string{"int", "int64"},
		},
		{
			Name:      "float32",
			CastTypes: []string{"float32", "float64"},
		},
		{
			Name:      "float64",
			CastTypes: []string{"float32", "float64"},
		},
	}
	templateFuncMap = template.FuncMap{
		"ToTitle": strings.Title,
	}
)

type sourceConfig struct {
	Alias string `yaml:"alias"`
	Path  string `yaml:"path"`
}

func (sc sourceConfig) StructureName() string {
	_, structureName, _ := parsePackageAndStructure(sc.Path)
	return structureName
}

type mapperConfig struct {
	Alias       string         `yaml:"alias,omitempty"`
	Destination sourceConfig   `yaml:"destination"`
	Sources     []sourceConfig `yaml:"source"`
	Mapping     []string       `yaml:"map"`
	Relations   []string       `yaml:"relations"`
}

func (mc mapperConfig) MapperName() string {
	prefix := mc.Alias
	if len(prefix) == 0 {
		prefix = mc.Destination.StructureName()
	}
	return fmt.Sprintf("%sMapper", prefix)
}

func (mc mapperConfig) ListMapperName() string {
	prefix := mc.Alias
	if len(prefix) == 0 {
		prefix = mc.Destination.StructureName()
	}
	return fmt.Sprintf("%sListMapper", prefix)
}

type config struct {
	path    string
	out     string
	Imports []importPackage `yaml:"imports"`
	Mappers []mapperConfig  `yaml:"mappers"`
}

func main() {
	configPath := flag.String("c", "mappers.yml", "Config file path")
	outPath := flag.String("o", "mappers_gen.go", "Out file path")
	flag.Parse()
	mappersConfig := loadConfig(*configPath)
	mappersConfig.out = *outPath
	err := os.MkdirAll(filepath.Dir(*outPath), os.ModePerm)
	f, err := os.Create(*outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	params, err := params(mappersConfig)
	if err != nil {
		log.Fatal(err)
	}
	packageTemplate, err := template.New("").Funcs(template.FuncMap{
		"ToTitle": strings.Title,
	}).Parse(mapperTmpl)
	err = packageTemplate.Execute(f, params)
	if err != nil {
		log.Fatal(err)
	}
}

func loadConfig(path string) *config {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal(fmt.Sprintf("Mapping configuration file %s not found", path))
	}
	mappersConfig := config{path: path}
	err = yaml.NewDecoder(bytes.NewReader(data)).Decode(&mappersConfig)
	if err != nil {
		log.Fatal("Mapping configuration file format error:", err.Error())
	}
	if err != nil {
		log.Fatal("Generated mappers file error")
	}
	return &mappersConfig
}

type src struct {
	Alias     string
	ShortPath string
	Fields    []field
}

type field struct {
	Name    string
	Ptr     bool
	TypeStr string
}

type mappingParams struct {
	MapperFuncName     string
	ListMapperFuncName string
	Dst                src
	SrcList            []src
	FieldMappingRules  []struct {
		DstFieldName string
		SrcAlias     string
		SrcShortPath string
		SrcFieldName string
		SrcFieldPtr  bool
		CastStr      string
		Casted       bool
	}
}

type importPackage struct {
	Alias string
	Path  string
}

func params(mappersConfig *config) (interface{}, error) {
	if !filepath.IsAbs(mappersConfig.path) {
		mappersConfig.path, _ = filepath.Abs(mappersConfig.path)
	}
	packageName := mapperFilePackage(mappersConfig.out)
	mappers := make([]mappingParams, 0, len(mappersConfig.Mappers)*2)
	for _, mapperConfig := range mappersConfig.Mappers {

		var dst src
		dst.Alias = mapperConfig.Destination.Alias
		dstMeta, err := parseStructure(pathUtil.Dir(mappersConfig.path), mapperConfig.Destination.Path)
		if err != nil {
			return nil, err
		}
		dst.ShortPath = shortPath(dstMeta)
		if err != nil {
			return nil, err
		}
		for _, f := range dstMeta.fields {
			dst.Fields = append(dst.Fields, struct {
				Name    string
				Ptr     bool
				TypeStr string
			}{
				Name:    f.name,
				Ptr:     isPtr(f.typeAST),
				TypeStr: typeStrValue(f.typeAST),
			})
		}

		var srcList []src
		for _, mapperSrc := range mapperConfig.Sources {
			srcMeta, err := parseStructure(pathUtil.Dir(mappersConfig.path), mapperSrc.Path)
			if err != nil {
				return nil, err
			}
			var fields []field
			for _, f := range srcMeta.fields {
				fields = append(fields, struct {
					Name    string
					Ptr     bool
					TypeStr string
				}{
					Name:    f.name,
					Ptr:     isPtr(f.typeAST),
					TypeStr: typeStrValue(f.typeAST),
				})
			}
			srcList = append(srcList, src{
				Alias:     mapperSrc.Alias,
				ShortPath: shortPath(srcMeta),
				Fields:    fields,
			})
		}

		fieldMappingRuleMap := map[string]struct {
			DstFieldName string
			SrcAlias     string
			SrcShortPath string
			SrcFieldName string
			SrcFieldPtr  bool
			CastStr      string
			Casted       bool
		}{}
		for _, dstField := range dst.Fields {
			for _, srcStruct := range srcList {
				for _, srcField := range srcStruct.Fields {
					if dstField.Name == srcField.Name {
						var fieldMappingRule struct {
							DstFieldName string
							SrcAlias     string
							SrcShortPath string
							SrcFieldName string
							SrcFieldPtr  bool
							CastStr      string
							Casted       bool
						}
						fieldMappingRule.DstFieldName = dstField.Name
						fieldMappingRule.SrcAlias = srcStruct.Alias
						fieldMappingRule.SrcShortPath = srcStruct.ShortPath
						fieldMappingRule.SrcFieldName = srcField.Name
						fieldMappingRule.SrcFieldPtr = srcField.Ptr
						fieldMappingRule.CastStr, fieldMappingRule.Casted = castDstField(srcStruct.Alias, srcField, dstField)
						fieldMappingRuleMap[dstField.Name] = fieldMappingRule
					}
				}
			}
		}

		for _, relation := range mapperConfig.Relations {
			var fieldMappingRule struct {
				DstFieldName string
				SrcAlias     string
				SrcShortPath string
				SrcFieldName string
				SrcFieldPtr  bool
				CastStr      string
				Casted       bool
			}

			_, dstFieldName, srcCastRow := parseRelation(relation)

			fieldMappingRule.DstFieldName = dstFieldName
			fieldMappingRule.CastStr, fieldMappingRule.Casted = srcCastRow, true

			usedSrc := searchUsedSrc(srcList, srcCastRow)
			if usedSrc != nil {
				usedField := searchUsedField(usedSrc, srcCastRow)
				if usedField != nil {
					fieldMappingRule.SrcAlias = usedSrc.Alias
					fieldMappingRule.SrcShortPath = usedSrc.ShortPath
					fieldMappingRule.SrcFieldName = usedField.Name
					fieldMappingRule.SrcFieldPtr = usedField.Ptr
				}
			}

			fieldMappingRuleMap[dstFieldName] = fieldMappingRule
		}

		var fieldMappingRules []struct {
			DstFieldName string
			SrcAlias     string
			SrcShortPath string
			SrcFieldName string
			SrcFieldPtr  bool
			CastStr      string
			Casted       bool
		}
		for _, fieldMappingRule := range fieldMappingRuleMap {
			fieldMappingRules = append(fieldMappingRules, fieldMappingRule)
		}

		mappers = append(mappers, mappingParams{
			MapperFuncName:     mapperConfig.MapperName(),
			ListMapperFuncName: mapperConfig.ListMapperName(),
			Dst:                dst,
			SrcList:            srcList,
			FieldMappingRules:  fieldMappingRules,
		})
	}

	importPackages := make([]importPackage, 0, len(importPackageAliasMap))
	for alias, packagePath := range importPackageAliasMap {
		importPackages = append(importPackages, importPackage{
			Alias: alias,
			Path:  packagePath,
		})
	}
	for i := range mappersConfig.Imports {
		importPackages = append(importPackages, importPackage{
			Alias: mappersConfig.Imports[i].Alias,
			Path:  importsPath(filepath.Dir(mappersConfig.path), mappersConfig.Imports[i].Path),
		})
	}
	return struct {
		Timestamp      time.Time
		ConfPath       string
		PackageName    string
		ImportPackages []importPackage
		Mappers        []mappingParams
		TypeToPtrList  []string
		TypesCastList  []struct {
			Name      string
			CastTypes []string
		}
	}{
		Timestamp:      time.Now(),
		ConfPath:       mappersConfig.path,
		PackageName:    packageName,
		ImportPackages: importPackages,
		TypeToPtrList:  typeToPtrList,
		TypesCastList:  typesCastList,
		Mappers:        mappers,
	}, nil
}

func importsPath(dir, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	if strings.HasPrefix(path, filepath.Join(os.Getenv("GOPATH"), "src")) {
		return strings.TrimPrefix(path, filepath.Join(os.Getenv("GOPATH"), "src")+string(os.PathSeparator))
	}
	return path
}

func searchUsedField(srcStruct *src, castRow string) *field {
	for _, srcField := range srcStruct.Fields {
		if castRow == fmt.Sprintf("%s.%s", srcStruct.Alias, srcField.Name) {
			return &srcField
		}
	}
	var res *field
	for _, srcField := range srcStruct.Fields {
		if strings.Contains(castRow, fmt.Sprintf("%s.%s", srcStruct.Alias, srcField.Name)) {
			if res == nil || len(res.Name) < len(srcField.Name) {
				res = &field{}
				*res = srcField
			}
		}
	}
	return res
}

func searchUsedSrc(srcList []src, srcCastRow string) *src {
	for _, srcStruct := range srcList {
		if strings.Contains(srcCastRow, srcStruct.Alias) {
			return &srcStruct
		}
	}
	return nil
}

func shortPath(meta *structMeta) string {
	packageAlias := getPackageAlias(meta.packagePath)
	if len(packageAlias) != 0 {
		return fmt.Sprintf("%s.%s", packageAlias, meta.name)
	}
	return meta.name
}

func castDstField(srcAlias string, srcField struct {
	Name    string
	Ptr     bool
	TypeStr string
}, dstField struct {
	Name    string
	Ptr     bool
	TypeStr string
}) (string, bool) {
	dstType := dstField.TypeStr
	srcType := srcField.TypeStr
	srcRow := fmt.Sprintf("%s.%s", srcAlias, srcField.Name)
	if dstType == srcType {
		return srcRow, true
	}
	if dstType != srcType {
		if "*"+dstType == srcType {
			srcRow = "*" + srcRow
			return srcRow, true
		} else {
			switch dstType {
			case "bool":
				switch srcType {
				case "*bool":
					return fmt.Sprintf("*%s", srcRow), true
				}
			case "string":
				switch srcType {
				case "*string":
					return fmt.Sprintf("*%s", srcRow), true
				}
			case "*bool":
				switch srcType {
				case "bool":
					return fmt.Sprintf("%sPtr(%s)", strings.Trim(dstType, "*"), srcRow), true
				}
			case "*string":
				switch srcType {
				case "string":
					return fmt.Sprintf("%sPtr(%s)", strings.Trim(dstType, "*"), srcRow), true
				}
			case "byte",
				"uint", "uint8", "uint16", "uint32", "uint64",
				"int", "int8", "int16", "int32", "int64",
				"float32", "float64":
				switch srcType {
				case "byte",
					"uint", "uint8", "uint16", "uint32", "uint64",
					"int", "int8", "int16", "int32", "int64",
					"float32", "float64":
					return fmt.Sprintf("%s(%s(%s))", dstType, dstType, srcRow), true
				case "*byte",
					"*uint", "*uint8", "*uint16", "*uint32", "*uint64",
					"*int", "*int8", "*int16", "*int32", "*int64",
					"*float32", "*float64":
					return fmt.Sprintf("%s(*%s)", dstType, srcRow), true
				}
			case "*byte",
				"*uint", "*uint8", "*uint16", "*uint32", "*uint64",
				"*int", "*int8", "*int16", "*int32", "*int64",
				"*float32", "*float64":
				switch srcType {
				case "byte",
					"uint", "uint8", "uint16", "uint32", "uint64",
					"int", "int8", "int16", "int32", "int64",
					"float32", "float64":
					return fmt.Sprintf("%sPtr(%s(%s))", strings.Trim(dstType, "*"), strings.Trim(dstType, "*"), srcRow), true
				case "*byte",
					"*uint", "*uint8", "*uint16", "*uint32", "*uint64",
					"*int", "*int8", "*int16", "*int32", "*int64",
					"*float32", "*float64":
					return fmt.Sprintf("%s(%s(%s))", strings.Trim(dstType, "*"), strings.Trim(dstType, "*"), srcRow), true
				}
			case "[]bool", "[]string", "[]byte",
				"[]uint", "[]uint8", "[]uint16", "[]uint32", "[]uint64",
				"[]int", "[]int8", "[]int16", "[]int32", "[]int64",
				"[]float32", "[]float64":
				switch srcType {
				case "[]bool", "[]string", "[]byte",
					"[]uint", "[]uint8", "[]uint16", "[]uint32", "[]uint64",
					"[]int", "[]int8", "[]int16", "[]int32", "[]int64",
					"[]float32", "[]float64":
					return fmt.Sprintf("%sArrTo%sArr(%s)", strings.Trim(srcType, "[]"), strings.Title(strings.Trim(dstType, "[]")), srcRow), true
				case "[]*bool", "[]*string", "[]*byte",
					"[]*uint", "[]*uint8", "[]*uint16", "[]*uint32", "[]*uint64",
					"[]*int", "[]*int8", "[]*int16", "[]*int32", "[]*int64",
					"[]*float32", "[]*float64":
					return fmt.Sprintf("%sPtrArrTo%sArr(%s)", strings.Trim(srcType, "[]*"), strings.Title(strings.Trim(dstType, "[]")), srcRow), true
				}
			case "[]*bool", "[]*string", "[]*byte",
				"[]*uint", "[]*uint8", "[]*uint16", "[]*uint32", "[]*uint64",
				"[]*int", "[]*int8", "[]*int16", "[]*int32", "[]*int64",
				"[]*float32", "[]*float64":
				switch srcType {
				case "[]bool", "[]string", "[]byte",
					"[]uint", "[]uint8", "[]uint16", "[]uint32", "[]uint64",
					"[]int", "[]int8", "[]int16", "[]int32", "[]int64",
					"[]float32", "[]float64":
					return fmt.Sprintf("%sArrTo%sPtrArr(%s)", strings.Trim(srcType, "[]"), strings.Title(strings.Trim(dstType, "[]*")), srcRow), true
				case "[]*bool", "[]*string", "[]*byte",
					"[]*uint", "[]*uint8", "[]*uint16", "[]*uint32", "[]*uint64",
					"[]*int", "[]*int8", "[]*int16", "[]*int32", "[]*int64",
					"[]*float32", "[]*float64":
					return fmt.Sprintf("%sPtrArrTo%sPtrArr(%s)", strings.Trim(srcType, "[]*"), strings.Title(strings.Trim(dstType, "[]*")), srcRow), true
				}
			}
		}
	}
	return srcRow, false
}

func isPtr(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.ArrayType:
		return true
	case *ast.MapType:
		return true
	case *ast.SelectorExpr:
		return isPtr(t.X)
	case *ast.StarExpr:
		return true
	case *ast.Ident:
		return false
	default:
		//log.Printf("Unknown %T type", t)
	}
	return false
}

func typeStrValue(node ast.Expr) string {
	switch t := node.(type) {
	case *ast.ArrayType:
		return fmt.Sprintf("[]%s", typeStrValue(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", typeStrValue(t.Key), typeStrValue(t.Value))
	case *ast.SelectorExpr:
		return typeStrValue(t.X)
	case *ast.StarExpr:
		return fmt.Sprintf("*%s", typeStrValue(t.X))
	case *ast.Ident:
		return t.Name
	default:
		//log.Printf("Unknown %T type", t)
	}
	return ""
}

func parseRelation(relation string) (dstAlias, dstField, srcCastRow string) {
	res := strings.Split(relation, ":")
	if len(res) != 2 {
		panic(fmt.Errorf("relation %s incorrect format", relation))
	}
	var dstRow string
	dstRow, srcCastRow = strings.TrimSpace(res[0]), strings.TrimSpace(res[1])
	res = strings.Split(dstRow, ".")
	if len(res) == 2 {
		dstAlias, dstField = strings.TrimSpace(res[0]), strings.TrimSpace(res[1])
	} else {
		dstField = dstRow
	}
	return
}

func mapperFilePackage(mapperFilePath string) string {
	absPath, err := filepath.Abs(mapperFilePath)
	if err != nil {
		panic(err)
	}
	return filepath.Base(filepath.Dir(absPath))
}

func parseStructure(dir, path string) (*structMeta, error) {
	switch path {
	case "byte", "bool", "string",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"int", "int8", "int16", "int32", "int64",
		"float32", "float64":
		return &structMeta{name: path}, nil
	}
	i := strings.LastIndex(path, ".")
	if i <= 0 {
		return nil, fmt.Errorf("source path \"%s\" incorrect", path)
	}
	filePath := path[:i] + ".go"
	structName := path[i+1:]

	fileSet := token.NewFileSet()
	if !strings.HasSuffix(filePath, ".go") {
		filePath = filePath + ".go"
	}
	data, err := ioutil.ReadFile(pathUtil.Join(dir, filePath))
	if err != nil {
		data, err = ioutil.ReadFile(filepath.Join(os.Getenv("GOPATH"), "src", filePath))
		if err != nil {
			return nil, fmt.Errorf("incorrect file path %s", filePath)
		}
	} else {
		if packagePath, err := filepath.Abs(pathUtil.Join(dir, filePath)); err == nil {
			filePath = strings.TrimPrefix(packagePath, filepath.Join(os.Getenv("GOPATH"), "src", string(os.PathSeparator)))
		}
	}
	fileAST, err := parser.ParseFile(fileSet, "", data, 0)
	if err != nil {
		return nil, err
	}

	//importsMap := make(map[string]string, 10)
	//for i := range fileAST.Imports {
	//	var packageAlias string
	//	if fileAST.Imports[i].Name != nil {
	//		packageAlias = fileAST.Imports[i].Name.Name
	//	} else {
	//		packageAlias = getPackageAlias(fileAST.Imports[i].Path.Value)
	//	}
	//	importsMap[packageAlias] = fileAST.Imports[i].Path.Value
	//}
	//log.Printf("%v", importsMap)

	var res *structMeta
	if o, exist := fileAST.Scope.Objects[structName]; exist {
		if ts, ok := o.Decl.(*ast.TypeSpec); ok {
			res = &structMeta{
				name:        ts.Name.Name,
				packagePath: parseImportPackagePath(filePath),
			}
			if s, ok := ts.Type.(*ast.StructType); ok {
				fields := make([]fieldMeta, 0, len(s.Fields.List))
				for i := range s.Fields.List {
					f := s.Fields.List[i]
					if len(f.Names) == 0 {
						continue
					}
					field := struct {
						name    string
						tag     string
						typeAST ast.Expr
					}{
						name: f.Names[0].Name,
					}
					if tag := f.Tag; tag != nil {
						field.tag = tag.Value
					}
					field.typeAST = f.Type
					fields = append(fields, field)
				}
				res.fields = fields
			}
		}
	}
	if res == nil {
		return nil, fmt.Errorf("structure %s not found in file %s", structName, filePath)
	}
	return res, nil
}

type fieldMeta struct {
	name    string
	tag     string
	typeAST ast.Expr
}

type structMeta struct {
	name        string
	packagePath string
	fields      []fieldMeta
}

func parseImportPackagePath(sourcePath string) string {
	return filepath.Dir(strings.TrimLeft(sourcePath, filepath.Join(os.Getenv("GOPATH"), "src")))
}

func parsePackageAndStructure(srcPath string) (string, string, error) {
	i := strings.LastIndex(srcPath, ".")
	if i <= 0 || i+1 >= len(srcPath) {
		return "", "", fmt.Errorf("src path \"%s\" incorrect format", srcPath)
	}
	return srcPath[:i], srcPath[i+1:], nil
}

func getPackageAlias(importPackagePath string) string {
	if strings.HasSuffix(importPackagePath, ".go") {
		importPackagePath = strings.TrimRight(importPackagePath, ".go")
	}
	i := strings.LastIndex(importPackagePath, "/")
	if i <= 0 || i+1 >= len(importPackagePath) {
		return importPackagePath
	}
	defaultAlias := importPackagePath[i+1:]
	alias := defaultAlias
	i = 1
	v, exist := importPackageAliasMap[alias]
	for exist {
		if v == importPackagePath {
			exist = false
		} else {
			alias = defaultAlias + strconv.Itoa(i)
			i++
			v, exist = importPackageAliasMap[alias]
		}
	}
	importPackageAliasMap[alias] = importPackagePath
	return alias
}
