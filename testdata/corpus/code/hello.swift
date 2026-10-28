struct Sample {
    let id: Int
    let name: String
    func greet() -> String { "\(id):\(name)" }
}

let item = Sample(id: 1, name: "qa")
print(item.greet())
