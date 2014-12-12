package goimpl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/rpc"
	"reflect"
	"strings"
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
)

func TestMethods(t *testing.T) {
	tc := []struct {
		opts        GenOpts
		expected    string
		shouldError bool
	}{
		{
			opts: GenOpts{
				PkgName:             "pkg",
				ImplName:            "*Impl",
				Inter:               reflect.TypeOf((*io.ReadCloser)(nil)).Elem(),
				NoNamedReturnValues: true,
			},
			expected: `package pkg

import (
	"errors"
)

type Impl struct{}

func (i *Impl) Close() error {
	panic(errors.New("Not implemented"))
}

func (i *Impl) Read(u []uint8) (int, error) {
	panic(errors.New("Not implemented"))
}
`,
		}, {
			opts: GenOpts{
				Existing: AlmostClientCodec{},
				Inter:    reflect.TypeOf((*rpc.ClientCodec)(nil)).Elem(),
			},
			expected: strings.Replace(`package goimpl

import (
	"errors"
	"net/rpc"
)

type AlmostClientCodec struct{}

// number of inputs: had 1, want 0
func (a AlmostClientCodec) Close() (err error) {
	panic(errors.New("Not implemented"))
}

func (a AlmostClientCodec) ReadResponseBody(i interface{}) (err error) {
	panic(errors.New("Not implemented"))
}

// inputs[0]: had 'interface {}' want '*rpc.Request'; inputs[1]: had '*rpc.Request' want 'interface {}'
func (a AlmostClientCodec) WriteRequest(r *rpc.Request, i interface{}) (err error) {
	panic(errors.New("Not implemented"))
}
`, "'", "`", -1),
		},
		{
			opts: GenOpts{
				PkgName:  "pkg",
				ImplName: "-bad name",
				Inter:    reflect.TypeOf((*fmt.Stringer)(nil)).Elem(),
			},
			shouldError: true,
		},
	}
	for _, c := range tc {
		var buf bytes.Buffer
		err := Generate(&c.opts, &buf)
		gen := buf.String()
		if gen != c.expected {
			d := diffmatchpatch.New()
			diff := d.DiffToDelta(d.DiffMain(gen, c.expected, false))
			t.Errorf("opts: %#v. expected:\n%s\n-----\nGot:\n%s\n-----\nDiff:\n%v", c.opts, c.expected, gen, diff)
		}
		if c.shouldError && err == nil || !c.shouldError && err != nil {
			t.Errorf("opts: %#v. err: %v", c.opts, err)
		}
	}
}

type AlmostClientCodec struct{}

// Wrong number of inputs.
func (a AlmostClientCodec) Close(int) error {
	panic(errors.New("Not implemented"))
}

// missing.
// func (a AlmostClientCodec) ReadResponseBody(interface{}) error {
// 	panic(errors.New("Not implemented"))
// }

// As it should be.
func (a AlmostClientCodec) ReadResponseHeader(*rpc.Response) error {
	panic(errors.New("Not implemented"))
}

// Inputs swapped.
func (a AlmostClientCodec) WriteRequest(interface{}, *rpc.Request) error {
	panic(errors.New("Not implemented"))
}
