using System;

sealed class Sample {
    public int Id { get; }
    public string Name { get; }
    public Sample(int id, string name) { Id = id; Name = name; }
    public string Greet() => $"{Id}:{Name}";
}

static class Program {
    static void Main() {
        Console.WriteLine(new Sample(1, "qa").Greet());
    }
}
