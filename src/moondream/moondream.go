package moondream

// #include <stdlib.h>
// #include "moondream.h"
import "C"
import "unsafe"

func Init() bool {
	c_model_dir := C.CString( "ai" )
	defer C.free( unsafe.Pointer( c_model_dir ) )
	return C.moondream_init( c_model_dir ) == 0
}

func Shutdown() {
	C.moondream_shutdown()
}

func DescribePhoto( path string ) string {
	c_path := C.CString( path )
	defer C.free( unsafe.Pointer( c_path ) )

	c_description := C.moondream_describe_photo( c_path )
	defer C.free( unsafe.Pointer( c_description ) )

	return C.GoString( c_description )
}
