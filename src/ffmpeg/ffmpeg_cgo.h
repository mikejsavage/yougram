#pragma once

#include <stdint.h>

struct FirstFrameResult {
	char Error[ 256 ];
	uint8_t * Rgb;
	int Width, Height;
};

#ifdef __cplusplus
extern "C"
#endif
struct FirstFrameResult FirstFrame( const char * path );
