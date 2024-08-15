package main

/*
#cgo LDFLAGS: -lm
#define STB_IMAGE_IMPLEMENTATION
#include "stb_image.h"
#define STB_IMAGE_RESIZE_IMPLEMENTATION
#include "stb_image_resize2.h"
#define STB_IMAGE_WRITE_IMPLEMENTATION
#include "stb_image_write.h"

extern void goStbWriteCallback( void * context, void * data, int size );
void stbWriteCallback( void * context, void * data, int size ) {
    goStbWriteCallback( context, data, size );
}
*/
import "C"
