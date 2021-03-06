package ffi

import (
	"reflect"
	"strings"
	"unsafe"
)

//go:generate go tool compile -asmhdr ffi.h ffi.go

type argtype uint16

const (
	type64     argtype = 0 // movq              64 bit
	typeS32    argtype = 1 // movlqsx    signed 32 bit
	typeU32    argtype = 2 // movlqzx  unsigned 32 bit
	typeS16    argtype = 3 // movwqsx    signed 16 bit
	typeU16    argtype = 4 // movwqzx  unsigned 16 bit
	typeS8     argtype = 5 // movbqsx    signed 8  bit
	typeU8     argtype = 6 // movbqzx  unsigned 8  bit
	typeDouble argtype = 7 // movsd             64 bit
	typeFloat  argtype = 8 // movss             32 bit
	typeUnused argtype = 0xFFFF
)

type argument struct {
	offset uint16
	t      argtype
}

// Spec is the callspec needed to do the actuall call
type Spec struct {
	fn      uintptr
	base    uintptr
	stack   []argument
	intargs [6]argument
	xmmargs [8]argument
	ret0    argument
	ret1    argument
	xmmret0 argument
	xmmret1 argument
	rax     uint8
}

var sliceOffset = reflect.TypeOf(reflect.SliceHeader{}).Field(0).Offset

func fieldToOffset(k reflect.StructField, t string) (argument, bool) {
	switch k.Type.Kind() {
	case reflect.Slice:
		return argument{uint16(k.Offset + sliceOffset), type64}, false
	case reflect.Int, reflect.Uint, reflect.Uint64, reflect.Int64, reflect.Ptr:
		return argument{uint16(k.Offset), type64}, false
	case reflect.Int32:
		return argument{uint16(k.Offset), typeS32}, false
	case reflect.Uint32:
		return argument{uint16(k.Offset), typeU32}, false
	case reflect.Int16:
		return argument{uint16(k.Offset), typeS16}, false
	case reflect.Uint16:
		return argument{uint16(k.Offset), typeU16}, false
	case reflect.Int8:
		return argument{uint16(k.Offset), typeS8}, false
	case reflect.Uint8, reflect.Bool:
		return argument{uint16(k.Offset), typeU8}, false
	case reflect.Float32:
		return argument{uint16(k.Offset), typeFloat}, true
	case reflect.Float64:
		return argument{uint16(k.Offset), typeDouble}, true
	}
	panic("Unknown Type")
}

// FIXME: we don't support stuff > 64 bit

// MakeSpec builds a call specification for the given arguments
func MakeSpec(fn uintptr, args interface{}) Spec {
	v := reflect.ValueOf(args)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()

	var spec Spec

	spec.fn = fn

	spec.ret0.t = typeUnused
	spec.ret1.t = typeUnused
	spec.xmmret0.t = typeUnused
	spec.xmmret1.t = typeUnused

	haveRet := false

	intreg := 0
	xmmreg := 0

ARGS:
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tags := strings.Split(f.Tag.Get("ffi"), ",")
		ret := false
		st := ""
		for _, tag := range tags {
			if tag == "ignore" {
				continue ARGS
			}
			if tag == "ret" {
				if haveRet == true {
					panic("Only one return argument allowed")
				}
				ret = true
				haveRet = true
				continue
			}
			if strings.HasPrefix(tag, "type=") {
				st = tag[5:]
			}
		}
		if ret {
			off, xmm := fieldToOffset(f, st)
			if xmm {
				spec.xmmret0 = off
			} else {
				spec.ret0 = off
			}
			// FIXME ret1/xmmret1! - only needed for types > 64 bit
			continue
		}
		off, xmm := fieldToOffset(f, st)
		if xmm {
			if xmmreg < 8 {
				spec.xmmargs[xmmreg] = off
				xmmreg++
			} else {
				spec.stack = append(spec.stack, off)
			}
		} else {
			if intreg < 6 {
				spec.intargs[intreg] = off
				intreg++
			} else {
				spec.stack = append(spec.stack, off)
			}
		}
	}
	for i := intreg; i < 6; i++ {
		spec.intargs[i].t = typeUnused
	}
	for i := xmmreg; i < 8; i++ {
		spec.xmmargs[i].t = typeUnused
	}
	spec.rax = uint8(xmmreg)
	return spec
}

// Call calls the given spec with the given arguments
func (spec Spec) Call(args unsafe.Pointer) {
	spec.base = uintptr(args)

	entersyscall()
	asmcgocall(unsafe.Pointer(asmcallptr), uintptr(unsafe.Pointer(&spec)))
	exitsyscall()

	if _Cgo_always_false {
		_Cgo_use(args)
		_Cgo_use(spec)
	}
}

//go:linkname _Cgo_always_false runtime.cgoAlwaysFalse
var _Cgo_always_false bool

//go:linkname _Cgo_use runtime.cgoUse
func _Cgo_use(interface{})

//go:linkname asmcgocall runtime.asmcgocall
func asmcgocall(unsafe.Pointer, uintptr) int32

//go:linkname entersyscall runtime.entersyscall
func entersyscall()

//go:linkname exitsyscall runtime.exitsyscall
func exitsyscall()

func asmcall()

//go:linkname x_cgo_init x_cgo_init
func x_cgo_init()

// force _cgo_init into the .data segment (instead of .bss), so our "linker" can overwrite its contents
//go:linkname _cgo_init _cgo_init
var _cgo_init = uintptr(10)

type emptyComplex64 struct {
	a complex64
}
type emptyComplex128 complex128

func init() {
	if _Cgo_always_false {
		x_cgo_init() // prevent x_cgo_init from being optimized out
	}
}

//go:linkname funcPC runtime.funcPC
func funcPC(f interface{}) uintptr

var asmcallptr = funcPC(asmcall)
