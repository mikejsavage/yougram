package ffmpeg

/*
#cgo CXXFLAGS: -std=c++20
#include <stdlib.h>
#include "ffmpeg_cgo.h"
*/
import "C"

import (
	"errors"
	"image"
	"unsafe"
)

func FirstFrame( path string ) ( *image.RGBA, error ) {
	c_path := C.CString( path )
	defer C.free( unsafe.Pointer( c_path ) )

	res := C.FirstFrame( c_path )
	if res.Rgb == nil {
		return nil, errors.New( C.GoString( &res.Error[ 0 ] ) )
	}
	defer C.free( unsafe.Pointer( res.Rgb ) )

	return &image.RGBA {
		Pix: C.GoBytes( unsafe.Pointer( res.Rgb ), res.Width * res.Height * 4 ),
		Stride: int( res.Width ) * 4,
		Rect: image.Rect( 0, 0, int( res.Width ), int( res.Height ) ),
	}, nil
}
