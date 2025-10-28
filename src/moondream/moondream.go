package moondream

// #include <stdlib.h>
// #include "moondream.h"
import "C"

import (
	"fmt"
	"unsafe"
)

var initialized bool

// TODO: turn off threads in moondream-zig, try to use the smaller model for less memory usage
func Init() {
	c_model_dir := C.CString( "ai" )
	defer C.free( unsafe.Pointer( c_model_dir ) )

	fmt.Printf( "Init AI photo tagging..." )
	initialized = C.moondream_init( c_model_dir ) == 0

	if !initialized {
		fmt.Println( " failed, download moondream.bin and moondream.json from XXX if you want it" )
	} else {
		fmt.Println( " ok" )
	}
}

func Shutdown() {
	if initialized {
		C.moondream_shutdown()
	}
}

func Ok() bool {
	return initialized
}

func DescribePhoto( jpeg []byte ) string {
	c_description := C.moondream_describe_photo( ( *C.uint8_t )( unsafe.Pointer( &jpeg[ 0 ] ) ), C.size_t( len( jpeg ) ) )
	defer C.free( unsafe.Pointer( c_description ) )

	return C.GoString( c_description )
}
