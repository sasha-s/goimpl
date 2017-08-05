goimpl [![Build Status](https://travis-ci.org/sasha-s/goimpl.svg?branch=master)](https://travis-ci.org/sasha-s/goimpl)
======

A tool to generate stub implementation of an interface.

The output is printed to stdout, errors (if any) &mdash; to stderr.
##Installation
```sh
go get github.com/sasha-s/goimpl/cmd/goimpl
```
## In a nutshell
```sh
goimpl io.ReadWriteCloser "*pkg.impl"
```

Would print

```go
package pkg

type impl struct{}

func (i *impl) Close() (err error) {
    return
}

func (i *impl) Read(u []uint8) (i1 int, err error) {
    return
}

func (i *impl) Write(u []uint8) (i1 int, err error) {
    return
}
```

Let's say you already have this (the `Writer` is almost `http.ResponseWriter`, but not quite):

```go
package w12

import "net/http"

type Writer struct{}

func (w Writer) Write(float64) (error, *int) {
    return nil, nil
}

func (Writer) Header() http.Header {
    return nil
}
```

Running

```sh
goimpl -existing http.ResponseWriter "w12.Writer"
```

Gives

```go
package w12

type Writer struct{}

// inputs[0]: had `float64` want `[]uint8`; outputs[0]: had `error` want `int`; outputs[1]: had `*int` want `error`
func (w Writer) Write(u []uint8) (i int, err error) {
	return
}

func (w Writer) WriteHeader(i int) {
	return
}
```

Here only the missing methods and the methods with wrong signature are generates.

## Usage
```
Usage: goimpl [flags] [import1] [import2...] package.interfaceTypeName [(*|&)][package2.]typeName
This would generate empty implementation of the interfaceTypeName.
  -existing=false: Would trigger generation of missing method for the existing type(struct). Note, that if you want to use a pointer receiver prefix the type with '&'.
  -goimports=true: Run goimports on the generated code. The generated code might not compile if this is not set.
  -named=true: Generate named return values. The genrated code might not compile if this is not set.
```
### Alternative(s)
[impl](https://github.com/josharian/impl)
* impl is parsing AST, goimpl is using reflection.
* impl is much faster and has better error reporting.
* goimpl generates complete code that (usually) compiles (package/ imports).
* goimpl can generate a minimum update for the exising implementation.
* goimpl can work with ambiguous package names.
