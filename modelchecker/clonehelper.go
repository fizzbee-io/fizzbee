package modelchecker

import (
	"github.com/jayaprabhakar/go-clone"
	"reflect"
	"unsafe"
)

func init() {
	t := reflect.TypeOf(reflect.Value{})
	found := false
	fields := t.NumField()

	for i := 0; i < fields; i++ {
		field := t.Field(i)

		if field.Type.Kind() == reflect.UnsafePointer {
			found = true
			reflectValuePtrOffset = field.Offset
			break
		}
	}

	if !found {
		panic("go-clone: fail to find internal ptr field in reflect.Value")
	}
}

const sizeOfPointers = unsafe.Sizeof((interface{})(0)) / unsafe.Sizeof(uintptr(0))
// interfaceData is the underlying data of an interface.
// As the reflect.Value's interfaceData method is deprecated,
// it may be broken in any Go release.
// It's better to create a custom to hold the data.
//
// The type of interfaceData fields must be poniters.
// It's a way to cheat Go compile to generate calls to write barrier
// when copying interfaces.
type interfaceData struct {
	_ [sizeOfPointers]unsafe.Pointer
}

var typeOfInterface = reflect.TypeOf((*interface{})(nil)).Elem()
// forceClearROFlag clears all RO flags in v to make v accessible.
// It's a hack based on the fact that InterfaceData is always available on RO data.
// This hack can be broken in any Go version.
// Don't use it unless we have no choice, e.g. copying func in some edge cases.
func forceClearROFlag(v reflect.Value) reflect.Value {
	var i interface{}
	indirect := 0

	// Save flagAddr.
	for v.CanAddr() {
		v = v.Addr()
		indirect++
	}

	v = v.Convert(typeOfInterface)
	nv := reflect.ValueOf(&i)
	*(*interfaceData)(unsafe.Pointer(nv.Pointer())) = parseReflectValue(v)
	cleared := nv.Elem().Elem()

	for indirect > 0 {
		cleared = cleared.Elem()
		indirect--
	}

	return cleared
}

var reflectValuePtrOffset uintptr

// parseReflectValue returns the underlying interface data in a reflect value.
// It assumes that v is an interface value.
func parseReflectValue(v reflect.Value) interfaceData {
	pv := (unsafe.Pointer)(uintptr(unsafe.Pointer(&v)) + reflectValuePtrOffset)
	ptr := *(*unsafe.Pointer)(pv)
	return *(*interfaceData)(ptr)
}



func roleResolveCloneFn(refs map[string]*Role) func(allocator *clone.Allocator, old reflect.Value, new reflect.Value) {
	return func(allocator *clone.Allocator, old, new reflect.Value) {
		var oldRole *Role
		//fmt.Println("old.CanInterface", old.CanInterface(), "old.CanSet", old.CanSet(), "old.CanAddr", old.CanAddr())
		if !old.CanInterface() {
			old = forceClearROFlag(old)
		}
		old = forceClearROFlag(old)
		if old.CanInterface() {
			oldRole = old.Interface().(*Role)
		} else if old.CanAddr() {
			oldRole = reflect.NewAt(old.Type(), unsafe.Pointer(old.UnsafeAddr())).Elem().Interface().(*Role)
		} else {
			panic("roleResolveCloneFn: oldRole is not accessible")
		}

		newRolePtr := new.Addr().Interface().(**Role)
		value, err := deepCloneStarlarkValue(oldRole, refs)
		if err != nil {
			panic(err)
		}
		*newRolePtr = value.(*Role)
	}
}
