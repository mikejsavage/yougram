#pragma once

#include <stdint.h>
#include <stddef.h>

struct FaceDetectContext;

struct FaceDetectContext * facedetect_load( const char * gguf_path );
void facedetect_capi_free( struct FaceDetectContext * ctx );

const char * facedetect_last_error( struct FaceDetectContext * ctx );

struct FaceDetectEmbedding {
	float x1, y1, x2, y2;
	float score;
	float embedding[ 512 ];
};

size_t facedetect_embed( struct FaceDetectContext * ctx, struct FaceDetectEmbedding * embeddings, size_t n, const uint8_t * pixels, uint32_t w, uint32_t h );
