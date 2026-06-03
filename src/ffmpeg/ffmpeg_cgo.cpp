#include "ffmpeg_cgo.h"

extern "C" {
#include "libavcodec/avcodec.h"
#include "libavformat/avformat.h"
#include "libswscale/swscale.h"
#include "libavutil/imgutils.h"
}

#define CONCAT_HELPER( a, b ) a##b
#define CONCAT( a, b ) CONCAT_HELPER( a, b )
#define COUNTER_NAME( x ) CONCAT( x, __COUNTER__ )

template< typename F >
struct ScopeExit {
	ScopeExit( F f_ ) : f( f_ ) { }
	~ScopeExit() { f(); }
	F f;
};

struct DeferHelper {
	template< typename F > ScopeExit< F > operator+( F f ) { return f; }
};

#define defer [[maybe_unused]] const auto & COUNTER_NAME( DEFER_ ) = DeferHelper() + [&]()

static FirstFrameResult FfmpegError( int err ) {
	FirstFrameResult res = { };
	av_strerror( err, res.Error, sizeof( res.Error ) );
	return res;
}

static FirstFrameResult StringError( const char * str ) {
	FirstFrameResult res = { };
	strcpy( res.Error, str );
	return res;
}

extern "C" FirstFrameResult FirstFrame( const char * path ) {
	printf( "%s\n", path );
	AVFormatContext * fmt_ctx = NULL;
	int ok = avformat_open_input( &fmt_ctx, path, NULL, NULL );
	if( ok < 0 ) {
		return FfmpegError( ok );
	}
	defer { avformat_close_input( &fmt_ctx ); };

	ok = avformat_find_stream_info( fmt_ctx, NULL );
	if( ok < 0 ) {
		return FfmpegError( ok );
	}

	int video_stream = -1;
	for( int i = 0; i < fmt_ctx->nb_streams; i++ ) {
		if( fmt_ctx->streams[ i ]->codecpar->codec_type == AVMEDIA_TYPE_VIDEO ) {
			video_stream = i;
			break;
		}
	}

	const AVCodec * decoder = avcodec_find_decoder( fmt_ctx->streams[ video_stream ]->codecpar->codec_id );
	if( decoder == NULL ) {
		return StringError( "can't find a decoder for this codec" );
	}

	AVCodecContext * dec_ctx = avcodec_alloc_context3( decoder );
	if( dec_ctx == NULL ) {
		return StringError( "avcodec_alloc_context3" );
	}
	defer { avcodec_free_context( &dec_ctx ); };

	ok = avcodec_parameters_to_context( dec_ctx, fmt_ctx->streams[ video_stream ]->codecpar );
	if( ok < 0 ) {
		return FfmpegError( ok );
	}

	ok = avcodec_open2( dec_ctx, decoder, NULL );
	if( ok < 0 ) {
		return FfmpegError( ok );
	}

	AVPacket * pkt = av_packet_alloc();
	if( pkt == NULL ) {
		return StringError( "av_packet_alloc" );
	}
	defer { av_packet_free( &pkt ); };

	AVFrame * frame = av_frame_alloc();
	if( frame == NULL ) {
		return StringError( "av_frame_alloc" );
	}
	defer { av_frame_free( &frame ); };

	bool done = false;
	while( !done ) {
		if( av_read_frame( fmt_ctx, pkt ) < 0 )
			break;
		defer { av_packet_unref( pkt ); };

		if( pkt->stream_index != video_stream )
			continue;

		ok = avcodec_send_packet( dec_ctx, pkt );
		if( ok < 0 ) {
			return FfmpegError( ok );
		}

		ok = avcodec_receive_frame( dec_ctx, frame );
		if( ok < 0 ) {
			if( ok == AVERROR( EAGAIN ) )
				continue;
			return FfmpegError( ok );
		}
		break;
	}

	uint8_t * rgb = ( uint8_t * ) malloc( av_image_get_buffer_size( AV_PIX_FMT_RGBA, frame->width, frame->height, 1 ) );
	if( frame == NULL ) {
		return StringError( "malloc" );
	}

	// TODO: error handling...
	uint8_t * dest_data[4] = { rgb, NULL, NULL, NULL };
	int dest_linesize[4] = { 4 * frame->width, 0, 0, 0 };
	struct SwsContext * sws_ctx = sws_getContext(
		frame->width, frame->height, AVPixelFormat( frame->format ),
		frame->width, frame->height, AV_PIX_FMT_RGBA,
		SWS_BILINEAR, NULL, NULL, NULL
	);
	sws_scale( sws_ctx, ( const uint8_t ** )frame->data, frame->linesize, 0,
		frame->height, dest_data, dest_linesize );
	sws_freeContext( sws_ctx );

	return ( struct FirstFrameResult ) {
		.Rgb = rgb,
		.Width = frame->width,
		.Height = frame->height,
	};
}
