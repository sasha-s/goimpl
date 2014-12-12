// A command line tool to generate stub implementation of an interface.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/tools/imports"
)

func usage() {
	fmt.Fprintln(os.Stderr, `Usage: goimpl [flags] [import1] [import2...] package.interfaceTypeName [(*|&)][package2.]typeName
This would generate empty implementation of the interfaceTypeName.`)
	flag.PrintDefaults()
	os.Exit(1)
}

var named = flag.Bool("named", false, "Generate named return values.")
var goimports = flag.Bool("goimports", true, "Run goimports on the generated code.")
var existing = flag.Bool("existing", false, "Would trigger generation of missing method for the existing type(struct). Note, that if you want to use a pointer receiver prefix the type with '&'.")
var verbose = flag.Bool("verbose", false, "print the generated code on error.")

func main() {
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() < 2 {
		usage()
	}
	args := flag.Args()
	n := len(args)
	extras := args[:n-2]
	a := args[n-2:]
	inter, typeName := a[0], a[1]
	opts := GenOpts{Inter: inter, NoGoImports: !*goimports, NoNamedReturnValues: !*named, Extra: extras}
	if !*existing {
		pi, err := parse(typeName)
		check(err)
		opts.ImplName = pi.ptr + pi.name
		opts.PkgName = pi.pkg
	} else {
		opts.Existing = typeName
		if !strings.HasSuffix(typeName, "}") && !strings.HasSuffix(typeName, ")") {
			// We need to create a instance of this type.
			// Let's it's a struct/slice.
			opts.Existing += "{}"
		}
	}

	buf := new(bytes.Buffer)
	check(tm.Execute(buf, opts))

	src, err := imports.Process("", buf.Bytes(), nil)
	check(err, "imports:", buf.String())

	check(run(src), "run:", string(src))
}

type parsedType struct {
	ptr  string
	pkg  string
	name string
}

func parse(tp string) (parsedType, error) {
	tp = strings.TrimSpace(tp)
	t := strings.TrimPrefix(tp, "*")
	ptr := ""
	if len(t) != len(tp) {
		ptr = "*"
	}

	parts := strings.Split(t, ".")
	var pkg, name string
	switch len(parts) {
	case 0:
		return parsedType{}, nil
	case 1:
		name = parts[0]
	case 2:
		pkg, name = parts[0], parts[1]
	default:
		return parsedType{}, fmt.Errorf("failed to parse ``%s`. Expected [package.]type.", t)
	}
	return parsedType{ptr, pkg, name}, nil
}

// GenOpts: code generation options.
type GenOpts struct {
	PkgName             string   // target package.
	ImplName            string   // type (struct) that would implement the interface.
	Inter               string   // Interface to implement.
	Existing            string   // Existing type that we want to implement the interface.
	NoNamedReturnValues bool     // Do not generate named return values. The generated code might not compiple if this is set.
	NoGoImports         bool     // No goimports if set. Faster. The generated code might not compile.
	Extra               []string // Extra imports.
}

func run(src []byte) error {
	tempDir, err := ioutil.TempDir("", "goimpl_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	// Generate a bootsrap.go that would generate the final code using reflection.
	tempFile := filepath.Join(tempDir, "bootsrap.go")
	err = ioutil.WriteFile(tempFile, src, 0600)
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "run", tempFile) // Maybe add `-a`?.
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func check(err error, extra ...interface{}) {
	if err != nil {
		m := err.Error()
		if strings.Contains(m, "bootsrap.go") {
			err = fmt.Errorf("Could not find the target interface : %v", err)
		} else if strings.Contains(m, "dummy.go") {
			err = fmt.Errorf("Something went wrong with the generated code: %v", err)
		}
		if !*verbose {
			if len(extra) > 0 {
				extra = extra[:len(extra)-1]
			}
		}
		if len(extra) > 0 {
			fmt.Fprintln(os.Stderr, err, extra)
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

const templateS = `
package main

import (
	"reflect"
	"github.com/sasha-s/goimpl"
	{{range .Extra}}"{{.}}"
	{{end}}
)

func main() {
	err := goimpl.Generate(
		&goimpl.GenOpts{
			Inter: reflect.TypeOf((*{{.Inter}})(nil)).Elem(),
			PkgName: "{{.PkgName}}",
			ImplName: "{{.ImplName}}",
			{{if .Existing}}Existing: {{.Existing}},{{end}}
			NoNamedReturnValues: {{.NoNamedReturnValues}},
			NoGoImports: {{.NoGoImports}},
			Extra : []string{ {{range .Extra}} "{{.}}", {{end}} },
		},
		os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(-1)
	}
}
`

var tm = template.New("bootstrap")

func init() {
	_, err := tm.Parse(templateS)
	if err != nil {
		panic(err)
	}
}
