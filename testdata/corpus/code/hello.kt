data class Sample(val id: Int, val name: String)

fun greet(s: Sample): String = "${s.id}:${s.name}"

fun main() {
    println(greet(Sample(1, "qa")))
}
