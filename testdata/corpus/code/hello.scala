object Hello {
  case class Sample(id: Int, name: String)
  def greet(s: Sample): String = s"${s.id}:${s.name}"
  def main(args: Array[String]): Unit = {
    println(greet(Sample(1, "qa")))
  }
}
