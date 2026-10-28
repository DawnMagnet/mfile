class Sample
  def initialize(name)
    @name = name
  end

  def greet
    "hello #{@name}"
  end
end

puts Sample.new("qa").greet
