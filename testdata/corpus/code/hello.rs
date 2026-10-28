fn answer(n: i32) -> i32 {
    n * 2
}

fn main() {
    let values = vec![1, 2, 3];
    let total: i32 = values.iter().map(|v| answer(*v)).sum();
    println!("total {total}");
}
