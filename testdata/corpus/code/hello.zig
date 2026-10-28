const std = @import("std");

fn answer(n: i32) i32 {
    return n * 2;
}

pub fn main() void {
    std.debug.print("answer {d}\n", .{answer(21)});
}
