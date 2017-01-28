// A tool to generate stub implementation of an interface.
package goimpl

import (
	"bytes"
	"errors"
	"fmt"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"reflect"
	"strings"
	"text/template"
	"unicode"

	"golang.org/x/net/context"
	"golang.org/x/tools/imports"
)

// GenOpts specifies code generation options.
type GenOpts struct {
	PkgName             string              // target package.
	ImplName            string              // type (struct) that would implement the interface.
	Inter               reflect.Type        // Interface to implement.
	Existing            interface{}         // Existing type that we want to implement the interface.
	NoNamedReturnValues bool                // Do not generate named return values. The generated code might not compiple if this is set.
	MethodBlacklist     map[string]struct{} // Would not generate the code for those methods.
	Comments            map[string]string   // Add comments to those methods in generated code.
	NoGoImports         bool                // No goimports if set. Faster. The generated code might not compile.
	Extra               []string            // Extra imports.
}

// Generate an empty implementation of the interface as specified in opts and write the result to out.
func Generate(opts *GenOpts, out io.Writer) error {
	if opts.MethodBlacklist == nil {
		opts.MethodBlacklist = map[string]struct{}{}
	}
	if opts.Comments == nil {
		opts.Comments = map[string]string{}
	}
	if err := opts.handleExisting(); err != nil {
		return err
	}
	if opts.PkgName == "" {
		opts.PkgName, _ = packageAndName(opts.Inter)
	}
	buf := new(bytes.Buffer)
	if err := tm.Execute(buf, opts); err != nil {
		return err
	}
	// Parse it back.
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, "dummy.go", buf, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("Error parsing generated code: %s", err.Error())
	}
	b := bytes.NewBuffer([]byte{})
	// Print.
	cfg := &printer.Config{
		Mode:     printer.UseSpaces | printer.TabIndent,
		Tabwidth: 8,
	}
	if err = cfg.Fprint(b, fset, astFile); err != nil {
		return err
	}
	var bts []byte
	if opts.NoGoImports {
		bts = b.Bytes()
	} else if bts, err = imports.Process("dummy.go", b.Bytes(), nil); err != nil {
		return errors.New("Error fixing imports: " + err.Error())
	}
	_, err = out.Write(bts)
	return err
}

func (opts *GenOpts) handleExisting() error {
	if opts.Existing == nil {
		return nil
	}
	if opts.ImplName != "" {
		return errors.New("only one of ImplName and existing should be set.")
	}
	if opts.PkgName != "" {
		return errors.New("only one of PkgName and existing should be set.")
	}
	et := reflect.TypeOf(opts.Existing)

	em := opts.Methods(et)
	for i, mtd := range em {
		em[i].Inputs = mtd.Inputs[1:]
	}
	mtds := opts.Methods(opts.Inter)
	eMap := toMap(em)
	rMap := toMap(mtds)
	for k, v := range rMap {
		w, ok := eMap[v.Name]
		if !ok {
			continue
		}
		d := w.Diff(*v)
		if d != "" {
			comm, _ := opts.Comments[k]
			if comm != "" {
				comm = comm + " "
			}
			opts.Comments[k] = comm + d
		} else {
			// Method exists and has a right signature.
			opts.MethodBlacklist[k] = struct{}{}
		}
	}
	if et.Kind() == reflect.Ptr {
		var name string
		opts.PkgName, name = packageAndName(et.Elem())
		opts.ImplName = "*" + name
	} else {
		opts.PkgName, opts.ImplName = packageAndName(et)
	}
	return nil
}

// Arg describes an argument of a method: either in or out.
type Arg struct {
	reflect.Type
	ArgName string // Name for a variable for this arg.
	Sep     string // Separator - empty if it the last arg in a list, comma otherwise.
}

// Method.
type Method struct {
	reflect.Method
	Inputs  []Arg
	Outputs []Arg
	Comment string
}

func toMap(m []Method) map[string]*Method {
	r := map[string]*Method{}
	for i := range m {
		r[m[i].Name] = &m[i]
	}
	return r
}

// Diff returns an empty string if the methods have same signature.
// Returns a description of the diff oftherwise.
func (m Method) Diff(other Method) string {
	if m.Name != other.Name {
		// Does not really happen.
		return fmt.Sprintf("names are different: %s != %s", m.Name, other.Name)
	}
	a := diff("input", m.Inputs, other.Inputs)
	b := diff("output", m.Outputs, other.Outputs)
	if a != "" && b != "" {
		return a + "; " + b
	}
	return a + b
}

func diff(n string, a, b []Arg) string {
	if len(a) != len(b) {
		return fmt.Sprintf("number of %ss: had %d, want %d", n, len(a), len(b))
	}
	ds := []string{}
	for i := range a {
		if a[i].String() != b[i].String() {
			ds = append(ds, fmt.Sprintf("%ss[%d]: had `%s` want `%s`", n, i, a[i].String(), b[i].String()))
		}
	}
	return strings.Join(ds, "; ")

}

// Methods populates a list of methods for a given reflect type (which is supposed to be an interface).
func (opts *GenOpts) Methods(it reflect.Type) []Method {
	m := make([]Method, 0, it.NumMethod())
	rec := opts.First(opts.ImplName)
	for i := 0; i < it.NumMethod(); i++ {
		name := it.Method(i).Name
		if _, ok := opts.MethodBlacklist[name]; !ok {
			mtd := opts.Method(rec, it.Method(i))
			if c, ok := opts.Comments[name]; ok {
				mtd.Comment = c
			}
			m = append(m, mtd)
		}
	}
	return m
}

// Method populates the Method struct.
// recName is a name of the receiver in the generated code.
func (opts *GenOpts) Method(recName string, ft reflect.Method) Method {
	cur := map[string]struct{}{recName: struct{}{}} // Current names. Start with the name of receiver.
	inp := make([]Arg, ft.Type.NumIn())
	last := len(inp) - 1
	for i := range inp {
		t := ft.Type.In(i)
		sep := ", "
		if i == last {
			sep = ""
		}
		inp[i] = Arg{Type: t, ArgName: opts.Short(t, cur), Sep: sep}
	}
	out := make([]Arg, ft.Type.NumOut())
	last = len(out) - 1
	for i := range out {
		t := ft.Type.Out(i)
		sep := ", "
		if i == last {
			sep = ""
		}
		out[i] = Arg{Type: t, ArgName: opts.Short(t, cur), Sep: sep}
	}
	return Method{Inputs: inp, Outputs: out, Method: ft}
}

// Clean keeps only letters.
func (opts *GenOpts) Clean(s string) string {
	rs := []rune(s)
	res := make([]rune, 0, len(rs))
	for _, r := range rs {
		if !unicode.IsLetter(r) {
			continue
		}
		res = append(res, r)
	}
	return string(res)
}

// Short returns a unique (in the current scope) name for the argument of type t.
func (opts *GenOpts) Short(t reflect.Type, cur map[string]struct{}) string {
	tt := t
	for tt.Kind() == reflect.Ptr || tt.Kind() == reflect.Slice {
		tt = tt.Elem()
	}
	pkg, name := packageAndName(tt)
	f := opts.First(name) // First letter.
	// Handle common types.
	switch {
	case t.ConvertibleTo(errorType):
		f = "err"
	case t.ConvertibleTo(ctxType):
		f = "ctx"
	default:
		n, clean := opts.lowerName(name)
		// Very short names.
		if len(n) <= 3 && string(n) != clean && string(n) != pkg {
			f = string(n)
		}
	}
	// Make sure the name is unique.
	name = f
	for c := 1; ; c++ {
		if _, ok := cur[name]; !ok {
			// Update the set of currently used names.
			cur[name] = struct{}{}
			return name
		}
		name = fmt.Sprintf("%s%d", f, c)
	}
}

func (opts *GenOpts) lowerName(s string) ([]rune, string) {
	parts := strings.Split(s, ".")
	if len(parts) == 0 {
		return []rune("u"), ""
	}
	clean := opts.Clean(parts[len(parts)-1])
	rs := []rune(clean)
	for i, r := range rs {
		rs[i] = unicode.ToLower(r)
	}
	return rs, clean
}

// First returns a first letter of s in lowercase.
func (GenOpts) First(s string) string {
	parts := strings.Split(s, ".")
	if len(parts) == 0 {
		return "u"
	}
	for _, r := range []rune(parts[len(parts)-1]) {
		if unicode.IsLetter(r) {
			return string([]rune{unicode.ToLower(r)})
		}
	}
	return "z"
}

// GetName of a type.
func (opts *GenOpts) GetName(t reflect.Type) string {
	name := t.Name()
	if name != "" {
		pkg, _ := packageAndName(t)
		// Handle the case the type is in the package we are generating code for.
		if pkg == "" || pkg == opts.PkgName {
			return name
		}
		return fmt.Sprintf("%s.%s", pkg, name)
	}
	switch t.Kind() {
	case reflect.Ptr:
		return fmt.Sprintf("*%s", opts.GetName(t.Elem()))
	case reflect.Map:
		return fmt.Sprintf("map[%s]%s", opts.GetName(t.Key()), opts.GetName(t.Elem()))
	case reflect.Slice:
		return fmt.Sprintf("[]%s", opts.GetName(t.Elem()))
	case reflect.Chan:
		return fmt.Sprintf("%s %s", t.ChanDir().String(), opts.GetName(t.Elem()))
	case reflect.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), opts.GetName(t.Elem()))
	case reflect.Func:
		inputs := make([]string, t.NumIn())
		for i := range inputs {
			inputs[i] = opts.GetName(t.In(i))
		}
		outputs := make([]string, t.NumOut())
		for i := range outputs {
			outputs[i] = opts.GetName(t.Out(i))
		}
		out := strings.Join(outputs, ", ")
		if len(outputs) > 1 {
			out = fmt.Sprintf("(%s)", out)
		}
		return fmt.Sprintf("func (%s) %s", strings.Join(inputs, ", "), out)
	default:
		return t.String()
	}
}

func packageAndName(t reflect.Type) (pkgName, name string) {
	pkgPath := t.PkgPath()
	if pkgPath == "" {
		return "", t.String()
	}
	parts := strings.Split(t.String(), ".")
	if len(parts) == 0 {
		return
	}
	if len(parts) > 1 {
		name = parts[1]
	}
	return parts[0], name
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()

const templateS = `
{{$R := .}}
package {{.PkgName}}

import (
	"errors"
	{{range .Extra}}"{{.}}"
	{{end}})
type {{.Clean .ImplName}} struct{}

{{$rec := .First .ImplName}}
{{range $R.Methods .Inter}}
{{if .Comment}}// {{ .Comment}} {{end}}
func ({{$rec}} {{$R.ImplName}}) {{.Name}} ({{range .Inputs}} {{.ArgName}} {{$R.GetName .}} {{.Sep}} {{end}}) ({{range .Outputs}} {{if not $R.NoNamedReturnValues}} {{.ArgName}} {{end}} {{$R.GetName .}} {{.Sep}} {{end}}) {
	panic(errors.New("{{$R.ImplName}}.{{.Name}} not implemented")) }
{{end}}
`

var _ = `{{if $R.NoNamedReturnValues}} {{range .Inputs}} _ {{if eq .Sep ""}} = {{else}} {{.Sep}} {{end}} {{end}} {{range .Inputs}} {{.ArgName}} {{.Sep}} {{end}}
	{{end}}`
var tm = template.New("impl")

func init() {
	_, err := tm.Parse(templateS)
	if err != nil {
		panic(err)
	}
}
