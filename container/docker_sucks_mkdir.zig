// zig build-exe -O ReleaseSmall -fstrip docker_sucks_mkdir.zig

const std = @import( "std" );

fn MakeDir( path: []const u8 ) !void {
	std.fs.makeDirAbsolute( path ) catch |err| switch (err) {
		error.PathAlreadyExists => return,
		else => return err,
	};
}

pub fn main() !void {
	try MakeDir( "/data" );
	try MakeDir( "/tmp" );
	try std.fs.deleteFileAbsolute( std.mem.span( std.os.argv[ 0 ] ) );
}
