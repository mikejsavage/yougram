package stb

/*
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include "stb_image.h"
#include "stb_image_resize2.h"
#include "stb_image_write.h"

void stbWriteCallback( void * context, void * data, int size );
*/
import "C"

import (
	"errors"
	"image"
	"os"
	"strings"
	"unsafe"
)

func StbLoadFile( f *os.File ) ( *image.RGBA, error ) {
	fd, err := C.dup( C.int( f.Fd() ) )
	if err != nil {
		return nil, err
	}

	c_mode := C.CString( "rb" )
	defer C.free( unsafe.Pointer( c_mode ) )
	c_file, err := C.fdopen( fd, c_mode )
	if err != nil {
		return nil, err
	}
	defer C.fclose( c_file )

	var w, h C.int
	data := C.stbi_load_from_file( c_file, &w, &h, nil, C.int( 4 ) )
	if data == nil {
		return nil, errors.New( C.GoString( C.stbi_failure_reason() ) )
	}
	defer C.stbi_image_free( unsafe.Pointer( data ) )

	return &image.RGBA {
		Pix: C.GoBytes( unsafe.Pointer( data ), w * h * 4 ),
		Stride: int( w ) * 4,
		Rect: image.Rect( 0, 0, int( w ), int( h ) ),
	}, nil
}

func StbLoad( jpg []byte ) ( *image.RGBA, error ) {
	var w, h C.int
	data := C.stbi_load_from_memory( ( *C.uchar )( unsafe.Pointer( &jpg[ 0 ] ) ), C.int( len( jpg ) ), &w, &h, nil, C.int( 4 ) )
	if data == nil {
		return nil, errors.New( C.GoString( C.stbi_failure_reason() ) )
	}
	defer C.stbi_image_free( unsafe.Pointer( data ) )

	return &image.RGBA {
		Pix: C.GoBytes( unsafe.Pointer( data ), w * h * 4 ),
		Stride: int( w ) * 4,
		Rect: image.Rect( 0, 0, int( w ), int( h ) ),
	}, nil
}

func StbResize( img *image.RGBA, w int, h int ) ( *image.RGBA ) {
	input_pixels := ( *C.uchar )( unsafe.Pointer( &img.Pix[ 0 ] ) )
	output_pixels := C.stbir_resize_uint8_srgb(
		input_pixels, C.int( img.Rect.Dx() ), C.int( img.Rect.Dy() ), C.int( img.Stride ),
		nil, C.int( w ), C.int( h ), C.int( 0 ),
		C.STBIR_RGBA )
	defer C.free( unsafe.Pointer( output_pixels ) )

	return &image.RGBA {
		Pix: C.GoBytes( unsafe.Pointer( output_pixels ), C.int( w * h * 4 ) ),
		Stride: w * 4,
		Rect: image.Rect( 0, 0, w, h ),
	}
}

func StbResizeAndCrop( img *image.RGBA, crop_x, crop_y, crop_w, crop_h, resized_w, resized_h int ) ( *image.RGBA ) {
	input_pixels := ( *C.uchar )( unsafe.Pointer( &img.Pix[ crop_y * img.Stride + crop_x * 4 ] ) )
	output_pixels := C.stbir_resize_uint8_srgb(
		input_pixels, C.int( crop_w ), C.int( crop_h ), C.int( img.Stride ),
		nil, C.int( resized_w ), C.int( resized_h ), C.int( 0 ),
		C.STBIR_RGBA )
	defer C.free( unsafe.Pointer( output_pixels ) )

	return &image.RGBA {
		Pix: C.GoBytes( unsafe.Pointer( output_pixels ), C.int( resized_w * resized_h * 4 ) ),
		Stride: resized_w * 4,
		Rect: image.Rect( 0, 0, resized_w, resized_h ),
	}
}

//export goStbWriteCallback
func goStbWriteCallback( context unsafe.Pointer, data unsafe.Pointer, size C.int ) {
	builder := ( *strings.Builder )( context )
	builder.Write( C.GoBytes( data, size ) )
}

func StbToJpg( img *image.RGBA, quality int ) ( []byte, error ) {
	var builder strings.Builder
	ok := C.stbi_write_jpg_to_func( ( *C.stbi_write_func )( C.stbWriteCallback ), unsafe.Pointer( &builder ), C.int( img.Rect.Dx() ), C.int( img.Rect.Dy() ), C.int( 4 ),
		unsafe.Pointer( &img.Pix[ 0 ] ), C.int( quality ) )
	if ok == 0 {
		return nil, errors.New( "stbi_write_jpg" )
	}

	return []byte( builder.String() ), nil
}

func StbWriteJpg( path string, img *image.RGBA, quality int ) error {
	c_path := C.CString( path )
	defer C.free( unsafe.Pointer( c_path ) )
	ok := C.stbi_write_jpg( c_path, C.int( img.Rect.Dx() ), C.int( img.Rect.Dy() ), C.int( 4 ),
		unsafe.Pointer( &img.Pix[ 0 ] ), C.int( quality ) )
	if ok == 0 {
		return errors.New( "stbi_write_jpg" )
	}
	return nil
}
