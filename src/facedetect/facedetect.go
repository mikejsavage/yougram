package facedetect

/*
#include <stdlib.h>
#include "facedetect.h"
*/
import "C"

import (
	"image"
	"errors"
	"unsafe"
)

type FaceDetectContext struct {
	Ctx *C.struct_FaceDetectContext
}

func Load( model_path string ) ( *FaceDetectContext, error ) {
	c_model_path := C.CString( model_path )
	defer C.free( unsafe.Pointer( c_model_path ) )

	ctx := C.facedetect_load( c_model_path )
	if ctx == nil {
		return nil, errors.New( "idk" )
	}

	return &FaceDetectContext { ctx }, nil
}

func Shutdown( ctx *FaceDetectContext ) {
	C.facedetect_capi_free( ctx.Ctx )
}

type Embedding struct {
	X1 float32
	X2 float32
	Y1 float32
	Y2 float32
	Score float32
	Embedding [512]float32
}

func Embed( ctx *FaceDetectContext, img *image.RGBA ) []Embedding {
	const max_faces = 1024
	c_embeddings := make( []C.struct_FaceDetectEmbedding, max_faces )

	pixels := ( *C.uchar )( unsafe.Pointer( &img.Pix[ 0 ] ) )
	n := C.facedetect_embed( ctx.Ctx, &c_embeddings[ 0 ], C.size_t( max_faces ), pixels, C.uint32_t( img.Rect.Dx() ), C.uint32_t( img.Rect.Dy() ) )

	results := make( []Embedding, n )
	for i := 0; i < int( n ); i++ {
		results[ i ] = Embedding {
			X1: float32( c_embeddings[ i ].x1 ),
			X2: float32( c_embeddings[ i ].x2 ),
			Y1: float32( c_embeddings[ i ].y1 ),
			Y2: float32( c_embeddings[ i ].y2 ),
			Score: float32( c_embeddings[ i ].score ),
		}

		for j := 0; j < len( c_embeddings[ i ].embedding ); j++ {
			results[ i ].Embedding[ j ] = float32( c_embeddings[ i ].embedding[ j ] )
		}
	}

	return results
}
