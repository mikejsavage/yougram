// zig build-exe -O ReleaseSmall -mcpu=baseline -fstrip prepare_container.zig

const std = @import( "std" );

fn MakeDir( path: []const u8 ) !void {
	std.fs.makeDirAbsolute( path ) catch |err| switch (err) {
		error.PathAlreadyExists => return,
		else => return err,
	};
}

pub fn main() !void {
	try MakeDir( "/data" );
        var sentinel = try std.fs.createFileAbsolute( "/data/container_volume_sentinel", .{} );
        sentinel.close();
	try MakeDir( "/tmp" );
	try std.fs.deleteFileAbsolute( std.mem.span( std.os.argv[ 0 ] ) );
}
